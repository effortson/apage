// Package audit defines the canonical audit event names (spec §15, §15.5) and
// the actor/resource vocabulary. Actual persistence lives in the store layer to
// avoid import cycles; the worker drains the queue and writes asynchronously
// (spec §19.7).
package audit

// Event names (spec §15 审计日志事件 + §15.5 新增审计事件).
const (
	InstanceCreated      = "instance.created"
	AgentConnected       = "agent.connected"
	AgentDisconnected    = "agent.disconnected"
	FileRegistered       = "file.registered"
	FileUploaded         = "file.uploaded"
	FileScanned          = "file.scanned"
	FileRejected         = "file.rejected"
	PreviewLinkCreated   = "preview_link.created"
	PreviewLinkAccessed  = "preview_link.accessed"
	PreviewLinkDenied    = "preview_link.denied"
	PreviewLinkRevoked   = "preview_link.revoked"
	PreviewLinkUpdated   = "preview_link.updated"
	FileExpired          = "file.expired"
	FileDeleted          = "file.deleted"
	CustomDomainVerified = "custom_domain.verified"
	CustomDomainFailed   = "custom_domain.failed"

	// Abuse governance (spec §15.5).
	AbuseReported         = "abuse.reported"
	AbuseFlaggedByScanner = "abuse.flagged_by_scanner"
	AbuseBlacklistHit     = "abuse.blacklist_hit"
	LinkFrozen            = "link.frozen"
	InstanceFrozen        = "instance.frozen"
	TenantSuspended       = "tenant.suspended"
	TakedownReceived      = "takedown.received"
	TakedownActioned      = "takedown.actioned"

	// Platform admin actions (spec §8).
	AdminLogin         = "admin.login"
	TenantTrustChanged = "tenant.trust_changed"
	TenantRestored     = "tenant.restored"
	AbuseActioned      = "abuse.actioned"

	// Account lifecycle.
	UserRegistered     = "user.registered"
	MemberInvited      = "member.invited"
	MemberRoleChanged  = "member.role_changed"
	MemberRemoved      = "member.removed"
	CredentialsRotated = "instance.credentials_rotated"
	TokenRevoked       = "instance.token_revoked"
)

// Actor types (spec §15 actor_type).
const (
	ActorUser           = "user"
	ActorInstanceAPIKey = "instance_api_key"
	ActorSystem         = "system"
	ActorAnonymous      = "anonymous"
	ActorAdmin          = "admin"
)

// Entry is a single audit record awaiting persistence.
type Entry struct {
	TenantID     string
	InstanceID   string
	Event        string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	IP           string
	UserAgent    string
	Reason       string
}
