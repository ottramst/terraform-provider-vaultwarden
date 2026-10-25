package models

type User struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Email         string         `json:"email"`
	Key           string         `json:"key"`
	PrivateKey    string         `json:"privateKey"`
	Organizations []Organization `json:"organizations,omitempty"`
	// UserEnabled is only returned by the admin API.
	UserEnabled *bool `json:"userEnabled,omitempty"`
}
