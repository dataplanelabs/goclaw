package protocol

import (
	"encoding/base64"
	"encoding/json"
)

// SecretKey is a base64-encoded secret key from Zalo login.
type SecretKey string

func (s SecretKey) Bytes() []byte {
	decoded, err := base64.StdEncoding.DecodeString(string(s))
	if err != nil {
		return nil
	}
	return decoded
}

// LoginInfo from getLoginInfo response (AES-CBC encrypted).
type LoginInfo struct {
	UID             string          `json:"uid"`
	ZPWEnk          string          `json:"zpw_enk"`
	ZpwWebsocket    []string        `json:"zpw_ws"`
	ZpwServiceMapV3 ZpwServiceMapV3 `json:"zpw_service_map_v3"`
}

// ZpwServiceMapV3 holds Zalo service endpoint URLs.
type ZpwServiceMapV3 struct {
	Chat      []string `json:"chat"`
	Group     []string `json:"group"`
	File      []string `json:"file"`
	Profile   []string `json:"profile"`
	GroupPoll []string `json:"group_poll"`
	Reaction  []string `json:"reaction"`
	// Only fields needed for GoClaw; Zalo returns many more.
}

// ServerInfo from getServerInfo response.
type ServerInfo struct {
	Settings *Settings `json:"settings"`
}

// UnmarshalJSON handles Zalo's typo: "setttings" (3 t's) vs "settings".
func (s *ServerInfo) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, k := range []string{"settings", "setttings"} {
		if v, ok := raw[k]; ok {
			return json.Unmarshal(v, &s.Settings)
		}
	}
	return nil
}

// Settings holds server-provided configuration.
type Settings struct {
	Features  Features          `json:"features"`
	Keepalive KeepaliveSettings `json:"keepalive"`
}

type Features struct {
	Socket SocketSettings `json:"socket"`
}

type SocketSettings struct {
	PingInterval     int                          `json:"ping_interval"`
	Retries          map[string]SocketRetryConfig `json:"retries"`
	CloseAndRetry    []int                        `json:"close_and_retry_codes"`
	RotateErrorCodes []int                        `json:"rotate_error_codes"`
}

type SocketRetryConfig struct {
	Max   *int  `json:"max,omitempty"`
	Times []int `json:"times"`
}

// UnmarshalJSON handles Zalo's OneOrMany pattern: value can be int or []int.
func (r *SocketRetryConfig) UnmarshalJSON(data []byte) error {
	type Alias struct {
		Max   *int            `json:"max,omitempty"`
		Times json.RawMessage `json:"times"`
	}
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	r.Max = a.Max
	// Try []int first
	if err := json.Unmarshal(a.Times, &r.Times); err != nil {
		// Try single int
		var single int
		if err2 := json.Unmarshal(a.Times, &single); err2 != nil {
			return err
		}
		r.Times = []int{single}
	}
	return nil
}

type KeepaliveSettings struct {
	AlwaysKeepalive   uint `json:"alway_keepalive"`
	KeepaliveDuration uint `json:"keepalive_duration"`
}

// --- Zalo API response types ---

// Response is the generic Zalo API response envelope.
type Response[T any] struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Data         T      `json:"data"`
}

// QRGeneratedData from QR code generation response.
type QRGeneratedData struct {
	Code  string `json:"code"`
	Image string `json:"image"` // base64-encoded PNG with data URI prefix
}

// QRScannedData from QR waiting-scan response.
type QRScannedData struct {
	Avatar      string `json:"avatar"`
	DisplayName string `json:"display_name"`
}

// QRUserInfo from getUserInfo response.
type QRUserInfo struct {
	Logged bool     `json:"logged"`
	Info   UserInfo `json:"info"`
}

// UserInfo holds basic Zalo user info.
type UserInfo struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// --- Poll types (faithful to zca-js src/apis/createPoll.ts + src/models). ---

// CreatePollOptions mirrors zca-js CreatePollOptions. Zero values are omitted
// from the encrypted payload so Zalo applies its own defaults.
type CreatePollOptions struct {
	Question          string   `json:"question"`
	Options           []string `json:"options"`
	ExpiredTime       int64    `json:"expired_time,omitempty"`       // ms; 0 = no expiration
	AllowMultiChoices bool     `json:"allow_multi_choices,omitempty"`
	AllowAddNewOption bool     `json:"allow_add_new_option,omitempty"`
	HideVotePreview   bool     `json:"is_hide_vote_preview,omitempty"`
	IsAnonymous       bool     `json:"is_anonymous,omitempty"`
}

// PollOption is a single answer slot on a poll.
type PollOption struct {
	OptionID   int64    `json:"option_id"`
	Content    string   `json:"content"`
	VotedUsers []string `json:"voted_users,omitempty"`
	VoteCount  int      `json:"vote_count"`
	Voted      bool     `json:"voted,omitempty"`
}

// PollDetail is the full poll-state response. Field shape inferred from
// zca-js usage; verify against a live response during Phase 2 (Locked may
// instead be encoded as `state: 1`).
type PollDetail struct {
	PollID            json.Number  `json:"poll_id"`
	Question          string       `json:"question"`
	Options           []PollOption `json:"options"`
	ExpiredTime       int64        `json:"expired_time"`
	PollType          int          `json:"poll_type"`
	AllowMultiChoices bool         `json:"allow_multi_choices"`
	AllowAddNewOption bool         `json:"allow_add_new_option"`
	HideVotePreview   bool         `json:"is_hide_vote_preview"`
	IsAnonymous       bool         `json:"is_anonymous"`
	GroupID           string       `json:"group_id"`
	CreatorID         string       `json:"creator_id"`
	CreatedTime       int64        `json:"created_time"`
	Locked            bool         `json:"locked,omitempty"`
}

// AddPollOptionsItem is one entry in addPollOptions's new_options payload.
type AddPollOptionsItem struct {
	Voted   bool   `json:"voted"`
	Content string `json:"content"`
}
