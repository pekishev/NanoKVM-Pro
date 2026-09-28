package proto

type GetHidModeRsp struct {
	Mode string `json:"mode"` // normal or hid-only
}

type SetHidModeReq struct {
	Mode string `validate:"required"` // normal or hid-only
}

type Macro struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

type GetMacrosRsp struct {
	Macros []Macro `json:"macros"`
}

type AddMacroReq struct {
	Name   string `json:"name" validate:"required,max=64"`
	Script string `json:"script" validate:"required,min=1,max=16384"`
}

type UpdateMacroReq struct {
	ID     string `json:"id" validate:"required"`
	Name   string `json:"name" validate:"required,max=64"`
	Script string `json:"script" validate:"required,min=1,max=16384"`
}

type DeleteMacroReq struct {
	ID string `json:"id" validate:"required"`
}
