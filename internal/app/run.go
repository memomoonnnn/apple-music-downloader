package app

import (
	"amdl/internal/amp-api"
	"amdl/internal/config"
	"amdl/internal/download"
	"amdl/internal/events"
	fairplayrip "amdl/internal/fairplay-rip"
	"encoding/json"
	"fmt"
	"github.com/spf13/pflag"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func Run() int {
	r := NewRunner(config.ConfigSet{})
	eventMode := flagValueFromArgs(os.Args[1:], "events") == "jsonl"
	if eventMode {
		output := os.Stdout
		sink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return 1
		}
		os.Stdout = sink
		defer func() {
			os.Stdout = output
			_ = sink.Close()
		}()
		r.Events = events.New(output)
		defer r.Events.Close()
		pflag.CommandLine = pflag.NewFlagSet("apple-music-downloader", pflag.ContinueOnError)
		pflag.CommandLine.SetOutput(io.Discard)
	}
	fail := func(message string) int {
		if r.Events != nil {
			r.Events.Emit("diagnostic", "", map[string]any{"level": "error", "code": "run_failed", "message": events.Redact(message)})
			r.Events.Emit("run_completed", "", map[string]any{"status": "failed", "completed": r.State.Counter.Success, "warnings": r.State.Counter.Unavailable + r.State.Counter.NotSong, "failures": r.State.Counter.Error + 1})
		} else {
			fmt.Fprintln(os.Stderr, message)
		}
		return 1
	}

	// --lite-server overrides the configured endpoint for this run. Scan the
	// raw args first so the /status check below uses the override too.
	r.Flags.LiteServerFlag = flagValueFromArgs(os.Args[1:], "lite-server")
	err := r.loadConfig()
	if err != nil {
		return fail(fmt.Sprintf("load Config failed: %v", err))
	}
	if r.Flags.LiteServerFlag != "" {
		r.Config.LiteServer = r.Flags.LiteServerFlag
	}
	var search_type string
	pflag.StringVar(&search_type, "search", "", "Search for 'album', 'song', or 'artist'. Provide query after flags.")
	pflag.BoolVar(&r.Flags.Atmos, "atmos", false, "Enable atmos download mode")
	pflag.BoolVar(&r.Flags.AAC, "aac", false, "Enable adm-aac download mode")
	pflag.BoolVar(&r.Flags.Select, "select", false, "Enable selective download")
	pflag.BoolVar(&r.Flags.ArtistSelect, "all-album", false, "Download all artist albums")
	pflag.BoolVar(&r.Flags.Debug, "debug", false, "Enable debug mode to show audio quality information")
	pflag.BoolVar(&r.Flags.PrintJSON, "json", false, "Output JSON summary at the end")
	pflag.BoolVar(&r.Flags.SaveM3U8, "save-m3u8-playlist", false, "Save M3U8 playlist file")
	eventsFormat := pflag.String("events", "", "Output Get Oudio events (jsonl)")
	pflag.StringVar(&r.Flags.LiteServerFlag, "lite-server", r.Config.LiteServer, "wrapper-lite HTTP API endpoint for this run")
	alac_max = pflag.Int("alac-max", r.Config.AlacMax, "Specify the max quality for download alac")
	atmos_max = pflag.Int("atmos-max", r.Config.AtmosMax, "Specify the max quality for download atmos")
	aac_type = pflag.String("aac-type", r.Config.AacType, "Select AAC type, aac aac-binaural aac-downmix")
	mv_audio_type = pflag.String("mv-audio-type", r.Config.MVAudioType, "Select MV audio type, atmos ac3 aac")
	mv_max = pflag.Int("mv-max", r.Config.MVMax, "Specify the max quality for download MV")

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [url1 url2 ...]\n", "[main | main.exe | go run main.go]")
		fmt.Fprintf(os.Stderr, "Search Usage: %s --search [album|song|artist] [query]\n", "[main | main.exe | go run main.go]")
		fmt.Println("\nOptions:")
		pflag.PrintDefaults()
	}

	if err := pflag.CommandLine.Parse(os.Args[1:]); err != nil {
		return fail(err.Error())
	}
	if *eventsFormat != "" && *eventsFormat != "jsonl" {
		return fail("unsupported events format")
	}
	if eventMode {
		if search_type != "" || r.Flags.Select || r.Flags.ArtistSelect || r.Flags.Debug {
			return fail("interactive options are unavailable with --events=jsonl")
		}
		r.Config.ExitOnError = true
	}
	if r.Flags.LiteServerFlag != "" {
		r.Config.LiteServer = r.Flags.LiteServerFlag
	}
	r.Config.AlacMax = *alac_max
	r.Config.AtmosMax = *atmos_max
	r.Config.AacType = *aac_type
	r.Config.MVAudioType = *mv_audio_type
	r.Config.MVMax = *mv_max

	args := pflag.Args()
	if len(args) == 0 {
		return fail("no URLs provided")
	}
	if eventMode && strings.Contains(args[0], "/artist/") {
		return fail("artist URLs are unavailable with --events=jsonl")
	}
	if regions, err := r.getLiteRegions(); err != nil {
		fmt.Println("Warning: failed to query lite-server /status:", err)
	} else {
		fmt.Printf("lite-server regions: %s\n", regions)
	}
	if err := download.Init(r.Config.Proxy); err != nil {
		return fail(fmt.Sprintf("proxy config error: %v", err))
	}
	if err := fairplayrip.Init(); err != nil {
		return fail(fmt.Sprintf("temari library error: %v", err))
	}
	token, err := ampapi.GetToken()
	if err != nil {
		if r.Config.AuthorizationToken != "" && r.Config.AuthorizationToken != "your-authorization-token" {
			token = strings.Replace(r.Config.AuthorizationToken, "Bearer ", "", -1)
		} else {
			return fail("Failed to get token.")
		}
	}

	if search_type != "" {
		if len(args) == 0 {
			fmt.Println("Error: --search flag requires a query.")
			pflag.Usage()
			return fail("search requires a query")
		}
		selectedUrl, err := r.handleSearch(search_type, args, token)
		if err != nil {
			fmt.Printf("\nSearch process failed: %v\n", err)
			return fail(err.Error())
		}
		if selectedUrl == "" {
			fmt.Println("\nExiting.")
			return fail("no search result selected")
		}
		os.Args = []string{selectedUrl}
	} else {
		os.Args = args
	}

	if strings.Contains(os.Args[0], "/artist/") {
		urlArtistName, urlArtistID, err := r.getUrlArtistName(os.Args[0], token)
		if err != nil {
			fmt.Println("Failed to get artistname.")
			return fail(err.Error())
		}
		r.Config.ArtistFolderFormat = strings.NewReplacer(
			"{UrlArtistName}", r.LimitString(urlArtistName),
			"{ArtistId}", urlArtistID,
		).Replace(r.Config.ArtistFolderFormat)
		albumArgs, err := r.checkArtist(os.Args[0], token, "albums")
		if err != nil {
			fmt.Println("Failed to get artist albums.")
			return fail(err.Error())
		}
		mvArgs, err := r.checkArtist(os.Args[0], token, "music-videos")
		if err != nil {
			fmt.Println("Failed to get artist music-videos.")
		}
		os.Args = append(albumArgs, mvArgs...)
	}
	if r.Events != nil {
		format := "alac"
		if r.Flags.Atmos {
			format = "atmos"
		} else if r.Flags.AAC {
			format = "aac"
		}
		r.Events.Emit("run_started", "", map[string]any{"format": format})
	}
	recordError := func(err error) {
		if r.Events != nil {
			r.Events.Emit("diagnostic", "", map[string]any{"level": "error", "code": "download_failed", "message": events.Redact(err.Error())})
		}
	}
	albumTotal := len(os.Args)
	for {
		for albumNum, urlRaw := range os.Args {
			fmt.Printf("Queue %d of %d: ", albumNum+1, albumTotal)
			var storefront, albumId string

			if strings.Contains(urlRaw, "/music-video/") {
				fmt.Println("Music Video")
				if r.Flags.Debug {
					continue
				}
				r.State.Counter.Total++
				storefront, albumId = checkUrl(urlRaw, "mv")
				if r.Events != nil {
					r.Events.Emit("item_started", albumId, map[string]any{"content": events.Content{ID: albumId, Kind: "music-videos"}})
				}
				failMV := func(err error) {
					r.State.Counter.Error++
					recordError(err)
					if r.Events != nil {
						r.Events.Emit("item_failed", albumId, map[string]any{"code": "download_failed", "message": events.Redact(err.Error())})
					}
				}
				if storefront == "" || albumId == "" {
					failMV(fmt.Errorf("invalid music video URL"))
					continue
				}
				if r.Config.LiteServer == "" {
					fmt.Println(": lite-server is not set, skip MV dl")
					failMV(fmt.Errorf("lite-server is not configured"))
					continue
				}
				mvSaveDir := strings.NewReplacer(
					"{ArtistName}", "",
					"{UrlArtistName}", "",
					"{ArtistId}", "",
				).Replace(r.Config.ArtistFolderFormat)
				if mvSaveDir != "" {
					mvSaveDir = filepath.Join(r.Config.MVSaveFolder, forbiddenNames.ReplaceAllString(mvSaveDir, "_"))
				} else {
					mvSaveDir = r.Config.MVSaveFolder
				}
				err := r.mvDownloader(albumId, mvSaveDir, token, storefront, nil)
				if err != nil {
					fmt.Println("\u26A0 Failed to dl MV:", err)
					failMV(err)
					continue
				}
				r.State.Counter.Success++
				if r.Events != nil {
					outputPath := ""
					if count := len(r.State.AddedTracks); count > 0 {
						outputPath = r.State.AddedTracks[count-1].Path
					}
					r.Events.Emit("item_completed", albumId, map[string]any{"output_path": outputPath})
				}
				continue
			}
			if strings.Contains(urlRaw, "/song/") {
				fmt.Printf("Song->")
				storefront, songId := checkUrl(urlRaw, "song")
				if storefront == "" || songId == "" {
					fmt.Println("Invalid song URL format.")
					r.State.Counter.Error++
					recordError(fmt.Errorf("invalid song URL"))
					continue
				}
				err := r.ripSong(songId, token, storefront, r.Config.MediaUserToken)
				if err != nil {
					fmt.Println("Failed to rip song:", err)
					r.State.Counter.Error++
					recordError(err)
				}
				continue
			}
			parse, err := url.Parse(urlRaw)
			if err != nil {
				fmt.Printf("Invalid URL: %v\n", err)
				r.State.Counter.Error++
				recordError(err)
				continue
			}
			var urlArg_i = parse.Query().Get("i")

			if strings.Contains(urlRaw, "/album/") {
				fmt.Println("Album")
				storefront, albumId = checkUrl(urlRaw, "album")
				err := r.ripAlbum(albumId, token, storefront, r.Config.MediaUserToken, urlArg_i)
				if err != nil {
					fmt.Println("Failed to rip album:", err)
					r.State.Counter.Error++
					recordError(err)
				}
			} else if strings.Contains(urlRaw, "/playlist/") {
				fmt.Println("Playlist")
				storefront, albumId = checkUrl(urlRaw, "playlist")
				err := r.ripPlaylist(albumId, token, storefront, r.Config.MediaUserToken)
				if err != nil {
					fmt.Println("Failed to rip playlist:", err)
					r.State.Counter.Error++
					recordError(err)
				}
			} else if strings.Contains(urlRaw, "/station/") {
				fmt.Printf("Station")
				storefront, albumId = checkUrl(urlRaw, "station")
				if len(r.Config.MediaUserToken) <= 50 {
					fmt.Println(": meida-user-token is not set, skip station dl")
					r.State.Counter.Error++
					recordError(fmt.Errorf("media-user-token is required for stations"))
					continue
				}
				err := r.ripStation(albumId, token, storefront, r.Config.MediaUserToken)
				if err != nil {
					fmt.Println("Failed to rip station:", err)
					r.State.Counter.Error++
					recordError(err)
				}
			} else {
				fmt.Println("Invalid type")
				r.State.Counter.Error++
				recordError(fmt.Errorf("invalid Apple Music URL type"))
			}
		}
		fmt.Printf("=======  [\u2714 ] Completed: %d/%d  |  [\u26A0 ] Warnings: %d  |  [\u2716 ] Errors: %d  =======\n", r.State.Counter.Success, r.State.Counter.Total, r.State.Counter.Unavailable+r.State.Counter.NotSong, r.State.Counter.Error)
		if r.State.Counter.Error == 0 {
			break
		} else if r.Config.ExitOnError {
			fmt.Println("Error detected, exiting...")
			break
		} else {
			fmt.Println("Error detected, press Enter to try again...")
			fmt.Scanln()
			fmt.Println("Start trying again...")
		}

		r.State.Counter = config.Counter{}
	}

	// Print JSON output
	if r.Flags.PrintJSON {
		jsonOutput, err := json.Marshal(r.State.AddedTracks)
		if err != nil {
			fmt.Println("Error generating JSON output:", err)
		} else {
			fmt.Println(string(jsonOutput))
		}
	}
	warnings := r.State.Counter.Unavailable + r.State.Counter.NotSong
	status := "completed"
	if r.State.Counter.Error > 0 {
		status = "failed"
	} else if warnings > 0 {
		status = "partial"
	}
	if r.Events != nil {
		r.Events.EndProgress()
		r.Events.Emit("run_completed", "", map[string]any{"status": status, "completed": r.State.Counter.Success, "warnings": warnings, "failures": r.State.Counter.Error})
	}
	if status != "completed" {
		return 1
	}
	return 0
}
