package domain

type Permission struct {
	ID   int64           `json:"id"`
	Name PermissionConst `json:"name"`
}

type PermissionConst string

const (
	PermMetricsRead PermissionConst = "metrics_read"

	PermServerRead  PermissionConst = "server_read"
	PermServerWrite PermissionConst = "server_write"

	PermMemberRead  PermissionConst = "member_read"
	PermMemberWrite PermissionConst = "member_write"

	PermAppRead  PermissionConst = "app_read"
	PermAppWrite PermissionConst = "app_write"

	PermAlertRead  PermissionConst = "alert_read"
	PermAlertWrite PermissionConst = "alert_write"
)
