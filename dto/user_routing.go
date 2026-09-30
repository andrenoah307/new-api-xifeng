package dto

type UserRoutingInfo struct {
	SourceGroup string `json:"source_group"`
	TargetGroup string `json:"target_group"`
	Model       string `json:"model"`
}
