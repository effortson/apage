package store

import (
	"encoding/json"
	"time"
)

// Tenant (spec §2).
type Tenant struct {
	TenantID   string    `json:"tenantId"`
	Name       string    `json:"name"`
	Plan       string    `json:"plan"`
	TrustLevel string    `json:"trustLevel"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
}

// User (spec §2).
type User struct {
	UserID          string     `json:"userId"`
	Email           string     `json:"email"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt"`
	AuthProvider    string     `json:"authProvider"`
	PasswordHash    string     `json:"-"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// Membership (spec §2).
type Membership struct {
	MembershipID string    `json:"membershipId"`
	UserID       string    `json:"userId"`
	TenantID     string    `json:"tenantId"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	Email        string    `json:"email,omitempty"` // joined for member listings
}

// Quota (spec §2).
type Quota struct {
	TenantID          string    `json:"tenantId"`
	Plan              string    `json:"plan"`
	InstanceLimit     int       `json:"instanceLimit"`
	StorageBytesLimit int64     `json:"storageBytesLimit"`
	StorageBytesUsed  int64     `json:"storageBytesUsed"`
	TunnelEgressLimit int64     `json:"tunnelEgressLimit"`
	TunnelEgressUsed  int64     `json:"tunnelEgressUsed"`
	CloudEgressLimit  int64     `json:"cloudEgressLimit"`
	CloudEgressUsed   int64     `json:"cloudEgressUsed"`
	CustomDomainLimit int       `json:"customDomainLimit"`
	CustomDomainUsed  int       `json:"customDomainUsed"`
	PeriodStart       time.Time `json:"periodStart"`
}

// Instance (spec §2, §26). Token hashes are never serialized.
type Instance struct {
	InstanceID   string     `json:"instanceId"`
	TenantID     string     `json:"tenantId"`
	AgentType    string     `json:"agentType"`
	AgentName    string     `json:"agentName"`
	Subdomain    string     `json:"subdomain"`
	Mode         string     `json:"mode"`
	Status       string     `json:"status"`
	AgentVersion string     `json:"agentVersion"`
	LastSeenAt   *time.Time `json:"lastSeenAt"`
	FrozenAt     *time.Time `json:"frozenAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// FileRef (spec §2 File Ref).
type FileRef struct {
	FileRef     string     `json:"fileRef"`
	InstanceID  string     `json:"instanceId"`
	DisplayName string     `json:"displayName"`
	Size        int64      `json:"size"`
	MimeType    string     `json:"mimeType"`
	ModifiedAt  *time.Time `json:"modifiedAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// File (spec §11).
type File struct {
	FileID        string     `json:"fileId"`
	TenantID      string     `json:"tenantId"`
	InstanceID    string     `json:"instanceId"`
	Status        string     `json:"status"`
	PreviewStatus string     `json:"previewStatus"`
	DisplayName   string     `json:"displayName"`
	Size          int64      `json:"size"`
	MimeType      string     `json:"mimeType"`
	StorageKey    string     `json:"-"`
	Visibility    string     `json:"visibility"`
	RejectReason  string     `json:"rejectReason,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// AccessPolicy mirrors spec §14 full schema.
type AccessPolicy struct {
	Type          string   `json:"type"` // public_token|password|account|ip_allowlist|single_use
	AllowDownload bool     `json:"allowDownload"`
	IPAllowlist   []string `json:"ipAllowlist,omitempty"`
	MaxViews      int      `json:"maxViews,omitempty"`
	SingleUse     bool     `json:"singleUse,omitempty"`
	Password      *struct {
		Enabled      bool   `json:"enabled"`
		Hash         string `json:"-"` // never serialized to clients
		AttemptLimit int    `json:"attemptLimit"`
	} `json:"password,omitempty"`
	Account *struct {
		Required         bool     `json:"required"`
		AllowedTenantIDs []string `json:"allowedTenantIds"`
		AllowedUserIDs   []string `json:"allowedUserIds"`
	} `json:"account,omitempty"`
}

// PreviewLink (spec §2 Preview Link). secret_hash is never serialized.
type PreviewLink struct {
	LinkID         string          `json:"linkId"`
	TenantID       string          `json:"tenantId"`
	InstanceID     string          `json:"instanceId"`
	FileRef        *string         `json:"fileRef,omitempty"`
	FileID         *string         `json:"fileId,omitempty"`
	Mode           string          `json:"mode"`
	DisplayName    string          `json:"displayName"`
	AccessPolicy   json.RawMessage `json:"accessPolicy"`
	ExpiresAt      *time.Time      `json:"expiresAt"`
	RevokedAt      *time.Time      `json:"revokedAt"`
	FrozenAt       *time.Time      `json:"frozenAt"`
	FrozenReason   string          `json:"frozenReason,omitempty"`
	LastAccessedAt *time.Time      `json:"lastAccessedAt"`
	ViewCount      int64           `json:"viewCount"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// CustomDomain (spec §5, §28).
type CustomDomain struct {
	DomainID      string     `json:"domainId"`
	TenantID      string     `json:"tenantId"`
	Domain        string     `json:"domain"`
	Status        string     `json:"status"`
	TXTValue      string     `json:"txtValue"`
	CertStatus    string     `json:"certStatus"`
	LastCheckedAt *time.Time `json:"lastCheckedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// AuditLog (spec §15).
type AuditLog struct {
	EventID      string    `json:"eventId"`
	TenantID     string    `json:"tenantId"`
	InstanceID   string    `json:"instanceId"`
	Event        string    `json:"event"`
	ActorType    string    `json:"actorType"`
	ActorID      string    `json:"actorId"`
	ResourceType string    `json:"resourceType"`
	ResourceID   string    `json:"resourceId"`
	IP           string    `json:"ip"`
	UserAgent    string    `json:"userAgent"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"createdAt"`
}

// AbuseReport (spec §15.5).
type AbuseReport struct {
	ReportID  string    `json:"reportId"`
	LinkID    string    `json:"linkId"`
	TenantID  string    `json:"tenantId"`
	Category  string    `json:"category"`
	Detail    string    `json:"detail"`
	SourceIP  string    `json:"-"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
