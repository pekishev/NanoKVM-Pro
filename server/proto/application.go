package proto

type GetVersionRsp struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	Source   string `json:"source"`
	Checking bool   `json:"checking"`
}

type GetUpdateCheckRsp struct {
	Enabled bool `json:"enabled"`
}

type SetUpdateCheckReq struct {
	Enable bool `validate:"omitempty"`
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
