package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"main/utils/structs"

	"gopkg.in/yaml.v2"
)

func TestParseDownloaderOptionsKeepsGetOudioArguments(t *testing.T) {
	config := structs.ConfigSet{
		AlacMax:     192000,
		AtmosMax:    2768,
		AacType:     "aac-lc",
		MVAudioType: "atmos",
		MVMax:       2160,
	}

	opts, err := ParseDownloaderOptions([]string{"--aac", "--song", "--events=jsonl", "https://music.apple.com/us/song/name/12345"}, config)
	if err != nil {
		t.Fatalf("ParseDownloaderOptions returned error: %v", err)
	}
	if !opts.AAC || !opts.Song || opts.Atmos {
		t.Fatalf("unexpected Get Oudio flags: AAC=%v Song=%v Atmos=%v", opts.AAC, opts.Song, opts.Atmos)
	}
	if opts.AACType != "aac-lc" || opts.AlacMax != 192000 || opts.AtmosMax != 2768 {
		t.Fatalf("config-backed defaults changed: %+v", opts)
	}
	if opts.EventsFormat != "jsonl" {
		t.Fatalf("events format changed: %q", opts.EventsFormat)
	}
	if got := strings.Join(opts.PositionalArgs, " "); got != "https://music.apple.com/us/song/name/12345" {
		t.Fatalf("unexpected positional args: %q", got)
	}
}

func TestParseDownloaderOptionsRejectsUnsupportedInteractiveFlags(t *testing.T) {
	if _, err := ParseDownloaderOptions(nil, structs.ConfigSet{}); err == nil {
		t.Fatal("expected missing URL error")
	}
	for _, flag := range []string{"--search", "--select", "--all-album", "--debug"} {
		if _, err := ParseDownloaderOptions([]string{flag, "https://music.apple.com/us/album/example/123456789"}, structs.ConfigSet{}); err == nil {
			t.Fatalf("expected %s to be rejected", flag)
		}
	}
}

func TestClassifyAppleMusicURL(t *testing.T) {
	cases := []struct {
		raw        string
		kind       AppleMusicURLKind
		storefront string
		id         string
		querySong  string
	}{
		{"https://music.apple.com/us/album/example/123456789?i=987654321", URLAlbum, "us", "123456789", "987654321"},
		{"https://music.apple.com/jp/song/example/987654321", URLSong, "jp", "987654321", ""},
		{"https://music.apple.com/us/playlist/example/pl.abc-123", URLPlaylist, "us", "pl.abc-123", ""},
		{"https://music.apple.com/us/station/example/ra.abc-123", URLStation, "us", "ra.abc-123", ""},
		{"https://music.apple.com/us/music-video/example/123456789", URLMusicVideo, "us", "123456789", ""},
	}
	for _, tc := range cases {
		got := ClassifyAppleMusicURL(tc.raw)
		if got.Kind != tc.kind || got.Storefront != tc.storefront || got.ID != tc.id || got.AlbumQuerySongID != tc.querySong {
			t.Fatalf("ClassifyAppleMusicURL(%q) = %+v", tc.raw, got)
		}
	}
}

func TestSelectedAlbumSongIDDocumentsSongReuse(t *testing.T) {
	if got, ok := SelectedAlbumSongID(true, "987654321"); !ok || got != "987654321" {
		t.Fatalf("expected --song album query id reuse, got %q ok=%v", got, ok)
	}
	if _, ok := SelectedAlbumSongID(false, "987654321"); ok {
		t.Fatal("album query id must not limit albums unless song mode is active")
	}
}

func TestConfigExampleKeepsGetOudioCompatibleFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("read config.yaml.example: %v", err)
	}
	for _, field := range []string{"convert-after-download:", "convert-format:", "ffmpeg-path:"} {
		if strings.Contains(string(data), field) {
			t.Fatalf("removed conversion field %q is still documented", field)
		}
	}
	var config structs.ConfigSet
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal config.yaml.example: %v", err)
	}
	if config.DecryptM3u8Port != "127.0.0.1:10020" || config.GetM3u8Port != "127.0.0.1:20020" || config.KeyServer != "127.0.0.1:40020" {
		t.Fatalf("wrapper port defaults changed: decrypt=%q get=%q", config.DecryptM3u8Port, config.GetM3u8Port)
	}
	if config.TemplateDecrypt {
		t.Fatal("standalone config must retain runv2 as its default decryptor")
	}
	if config.AlacSaveFolder == "" || config.AacSaveFolder == "" || config.AtmosSaveFolder == "" {
		t.Fatalf("download folders must remain configured: %+v", config)
	}
	if config.AacType == "" || config.AlacMax == 0 || config.AtmosMax == 0 {
		t.Fatalf("quality defaults must remain configured: %+v", config)
	}
	if config.EmbedLrc != true || config.EmbedCover != true {
		t.Fatalf("metadata defaults changed: embed-lrc=%v embed-cover=%v", config.EmbedLrc, config.EmbedCover)
	}
}

func TestExitAndRedactionContracts(t *testing.T) {
	if !ShouldExitAfterErrors(1, true) {
		t.Fatal("exit-on-error should terminate when errors are present")
	}
	if ShouldExitAfterErrors(1, false) || ShouldExitAfterErrors(0, true) {
		t.Fatal("exit-on-error contract changed")
	}
	redacted := RedactSensitiveText(`authorization-token: "Bearer abc.def.ghi" media-user-token: "secret-token" x-apple-music-user-token=another-secret`)
	for _, secret := range []string{"abc.def.ghi", "secret-token", "another-secret"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("secret %q was not redacted from %q", secret, redacted)
		}
	}
}

func TestTemplateDecryptContractFollowsConfiguration(t *testing.T) {
	if ShouldUseTemplateDecrypt(structs.ConfigSet{}) {
		t.Fatal("template decrypt must remain opt-in")
	}
	if !ShouldUseTemplateDecrypt(structs.ConfigSet{TemplateDecrypt: true}) {
		t.Fatal("template decrypt must be selected when explicitly enabled")
	}
}
