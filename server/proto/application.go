package proto

type GetVersionRsp struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

type UploadReleaseRsp struct {
	Version string `json:"version"`
}

type GetPreviewRsp struct {
	Enabled bool `json:"enabled"`
}

type SetPreviewReq struct {
	Enable bool `validate:"omitempty"`
}
