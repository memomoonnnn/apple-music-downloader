package task

import (
	"errors"
	"fmt"

	"main/utils/ampapi"
)

type Station struct {
	Storefront string
	ID         string

	SaveDir   string
	SaveName  string
	Codec     string
	CoverPath string

	Language string
	Resp     ampapi.StationResp
	Type     string
	Name     string
	Tracks   []Track
}

func NewStation(st string, id string) *Station {
	a := new(Station)
	a.Storefront = st
	a.ID = id
	//fmt.Println("Album created")
	return a

}

func (a *Station) GetResp(mutoken, token, l string) error {
	var err error
	a.Language = l
	resp, err := ampapi.GetStationResp(a.Storefront, a.ID, a.Language, token)
	if err != nil {
		return errors.New("error getting station response")
	}
	a.Resp = *resp
	//简化高频调用名称
	a.Type = a.Resp.Data[0].Attributes.PlayParams.Format
	a.Name = a.Resp.Data[0].Attributes.Name
	if a.Type != "tracks" {
		return nil
	}
	tracksResp, err := ampapi.GetStationNextTracks(a.ID, mutoken, a.Language, token)
	if err != nil {
		return errors.New("error getting station tracks response")
	}
	//fmt.Println("Getting album response")
	//从resp中的Tracks数据中提取trackData信息到新的Track结构体中
	for i, trackData := range tracksResp.Data {
		albumResp, err := ampapi.GetAlbumRespByHref(trackData.Href, a.Language, token)
		if err != nil {
			fmt.Println("Error getting album response:", err)
			continue
		}
		albumLen := len(albumResp.Data[0].Relationships.Tracks.Data)
		a.Tracks = append(a.Tracks, Track{
			ID:         trackData.ID,
			Type:       trackData.Type,
			Name:       trackData.Attributes.Name,
			Language:   a.Language,
			Storefront: a.Storefront,

			//SaveDir:   filepath.Join(a.SaveDir, a.SaveName),
			//Codec:     a.Codec,
			TaskNum:   i + 1,
			TaskTotal: len(tracksResp.Data),
			M3u8:      trackData.Attributes.ExtendedAssetUrls.EnhancedHls,
			WebM3u8:   trackData.Attributes.ExtendedAssetUrls.EnhancedHls,
			//CoverPath: a.CoverPath,

			Resp:      trackData,
			PreType:   "stations",
			DiscTotal: albumResp.Data[0].Relationships.Tracks.Data[albumLen-1].Attributes.DiscNumber,
			PreID:     a.ID,
			AlbumData: albumResp.Data[0],
		})
		a.Tracks[i].PlaylistData.Attributes.Name = a.Name
		a.Tracks[i].PlaylistData.Attributes.ArtistName = "Apple Music Station"
	}
	return nil
}

func (a *Station) GetArtwork() string {
	return a.Resp.Data[0].Attributes.Artwork.URL
}
