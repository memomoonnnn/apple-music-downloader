package contract

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"main/utils/structs"

	"github.com/spf13/pflag"
)

type AppleMusicURLKind string

const (
	URLInvalid    AppleMusicURLKind = "invalid"
	URLAlbum      AppleMusicURLKind = "album"
	URLPlaylist   AppleMusicURLKind = "playlist"
	URLSong       AppleMusicURLKind = "song"
	URLMusicVideo AppleMusicURLKind = "music-video"
	URLStation    AppleMusicURLKind = "station"
)

type AppleMusicURLTarget struct {
	Kind             AppleMusicURLKind
	Storefront       string
	ID               string
	AlbumQuerySongID string
}

type DownloaderOptions struct {
	Atmos          bool
	AAC            bool
	Song           bool
	PrintJSON      bool
	EventsFormat   string
	SaveM3U8       bool
	AlacMax        int
	AtmosMax       int
	AACType        string
	MVAudioType    string
	MVMax          int
	PositionalArgs []string
}

func ParseDownloaderOptions(args []string, config structs.ConfigSet) (DownloaderOptions, error) {
	var opts DownloaderOptions
	flags := pflag.NewFlagSet("apple-music-downloader", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&opts.Atmos, "atmos", false, "Enable atmos download mode")
	flags.BoolVar(&opts.AAC, "aac", false, "Enable adm-aac download mode")
	flags.BoolVar(&opts.Song, "song", false, "Enable single song download mode")
	flags.BoolVar(&opts.PrintJSON, "json", false, "Output JSON summary at the end")
	flags.StringVar(&opts.EventsFormat, "events", "", "Output Get Oudio events (jsonl)")
	flags.BoolVar(&opts.SaveM3U8, "save-m3u8-playlist", false, "Save M3U8 playlist file")
	flags.IntVar(&opts.AlacMax, "alac-max", config.AlacMax, "Specify the max quality for download alac")
	flags.IntVar(&opts.AtmosMax, "atmos-max", config.AtmosMax, "Specify the max quality for download atmos")
	flags.StringVar(&opts.AACType, "aac-type", config.AacType, "Select AAC type, aac aac-binaural aac-downmix")
	flags.StringVar(&opts.MVAudioType, "mv-audio-type", config.MVAudioType, "Select MV audio type, atmos ac3 aac")
	flags.IntVar(&opts.MVMax, "mv-max", config.MVMax, "Specify the max quality for download MV")
	if err := flags.Parse(args); err != nil {
		return DownloaderOptions{}, err
	}
	opts.PositionalArgs = flags.Args()
	if len(opts.PositionalArgs) == 0 {
		return DownloaderOptions{}, fmt.Errorf("no URLs provided")
	}
	return opts, nil
}

func ClassifyAppleMusicURL(raw string) AppleMusicURLTarget {
	switch {
	case strings.Contains(raw, "/music-video/"):
		storefront, id := matchURL(raw, `^(?:https:\/\/(?:beta\.music|music)\.apple\.com\/(\w{2})(?:\/music-video|\/music-video\/.+))\/(?:id)?(\d[^\D]+)(?:$|\?)`)
		return AppleMusicURLTarget{Kind: URLMusicVideo, Storefront: storefront, ID: id}
	case strings.Contains(raw, "/song/"):
		storefront, id := matchURL(raw, `^(?:https:\/\/(?:beta\.music|music|classical\.music)\.apple\.com\/(\w{2})(?:\/song|\/song\/.+))\/(?:id)?(\d[^\D]+)(?:$|\?)`)
		return AppleMusicURLTarget{Kind: URLSong, Storefront: storefront, ID: id}
	case strings.Contains(raw, "/album/"):
		storefront, id := matchURL(raw, `^(?:https:\/\/(?:beta\.music|music|classical\.music)\.apple\.com\/(\w{2})(?:\/album|\/album\/.+))\/(?:id)?(\d[^\D]+)(?:$|\?)`)
		return AppleMusicURLTarget{Kind: URLAlbum, Storefront: storefront, ID: id, AlbumQuerySongID: AlbumQuerySongID(raw)}
	case strings.Contains(raw, "/playlist/"):
		storefront, id := matchURL(raw, `^(?:https:\/\/(?:beta\.music|music|classical\.music)\.apple\.com\/(\w{2})(?:\/playlist|\/playlist\/.+))\/(?:id)?(pl\.[\w-]+)(?:$|\?)`)
		return AppleMusicURLTarget{Kind: URLPlaylist, Storefront: storefront, ID: id}
	case strings.Contains(raw, "/station/"):
		storefront, id := matchURL(raw, `^(?:https:\/\/(?:beta\.music|music)\.apple\.com\/(\w{2})(?:\/station|\/station\/.+))\/(?:id)?(ra\.[\w-]+)(?:$|\?)`)
		return AppleMusicURLTarget{Kind: URLStation, Storefront: storefront, ID: id}
	default:
		return AppleMusicURLTarget{Kind: URLInvalid}
	}
}

func AlbumQuerySongID(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("i")
}

func SelectedAlbumSongID(songMode bool, querySongID string) (string, bool) {
	if !songMode || querySongID == "" {
		return "", false
	}
	return querySongID, true
}

func ShouldExitAfterErrors(errorCount int, exitOnError bool) bool {
	return errorCount > 0 && exitOnError
}

func ShouldUseTemplateDecrypt(config structs.ConfigSet) bool {
	return config.TemplateDecrypt
}

func RedactSensitiveText(input string) string {
	if input == "" {
		return input
	}
	patterns := []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?i)(authorization-token\s*[:=]\s*["']?)(?:Bearer\s+)?[^\s"',]+`), `${1}<redacted>`},
		{regexp.MustCompile(`(?i)(media-user-token\s*[:=]\s*["']?)[^\s"',]+`), `${1}<redacted>`},
		{regexp.MustCompile(`(?i)(x-apple-music-user-token\s*[:=]\s*["']?)[^\s"',]+`), `${1}<redacted>`},
		{regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+`), `${1}<redacted>`},
	}
	out := input
	for _, pattern := range patterns {
		out = pattern.re.ReplaceAllString(out, pattern.repl)
	}
	return out
}

func matchURL(raw string, pattern string) (string, string) {
	matches := regexp.MustCompile(pattern).FindAllStringSubmatch(raw, -1)
	if matches == nil {
		return "", ""
	}
	return matches[0][1], matches[0][2]
}
