package control

type User struct {
	ID         string `json:"id"`
	Login      string `json:"login"`
	Name       string `json:"display_name"`
	Role       string `json:"role"`
	Enabled    bool   `json:"enabled"`
	MustChange bool   `json:"must_change_password"`
	Version    int    `json:"-"`
	Hash       string `json:"-"`
}

type Login struct {
	Hash, UserID, CSRF string
	Version            int
	Created, Active    int64
}

type Target struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Environment   string `json:"environment"`
	Enabled       bool   `json:"enabled"`
	JumpAccountID string `json:"jump_account_id,omitempty"`
}

type Account struct {
	ID        string `json:"id"`
	TargetID  string `json:"target_id"`
	Username  string `json:"ssh_username"`
	AuthType  string `json:"auth_type"`
	Enabled   bool   `json:"enabled"`
	Sharing   bool   `json:"sharing_enabled"`
	Recording bool   `json:"recording_enabled"`
}

type Grant struct {
	UserID    string `json:"user_id"`
	AccountID string `json:"target_account_id"`
	Connect   bool   `json:"can_connect"`
	View      bool   `json:"can_view_shared"`
}

type Share struct {
	ID         string   `json:"id"`
	SessionID  string   `json:"session_id"`
	Owner      string   `json:"created_by_user_id"`
	LoginHash  string   `json:"-"`
	Recipients []string `json:"recipient_user_ids"`
	Expires    int64    `json:"expires_at"`
	// Persisted separately from the public JSON representation.
}

type Log struct {
	ID        string `json:"id"`
	Owner     string `json:"owner_user_id,omitempty"`
	AccountID string `json:"target_account_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"user"`
	Started   int64  `json:"connected_at"`
	Ended     *int64 `json:"disconnected_at"`
	Error     string `json:"error,omitempty"`
	Reason    string `json:"termination_reason,omitempty"`
	Recording bool   `json:"has_recording"`
	Path      string `json:"-"`
}

type Audit struct {
	ID       int64  `json:"id"`
	Actor    string `json:"actor_user_id"`
	Action   string `json:"action"`
	Resource string `json:"resource_id"`
	Reason   string `json:"reason"`
	At       int64  `json:"created_at"`
}
