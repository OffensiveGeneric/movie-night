package setup

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Code is everything the guest needs to pair with the host, in one string
// the host can text over: the host's Syncthing device, the folders it
// shares, and the Syncplay room.
type Code struct {
	Device  string       `json:"d"`
	Name    string       `json:"n"`
	Server  string       `json:"s"` // Syncplay server, host:port
	Room    string       `json:"r"`
	Folders []CodeFolder `json:"f"`
}

type CodeFolder struct {
	ID    string `json:"i"`
	Label string `json:"l"`
}

const codePrefix = "MN1-"

func (c Code) String() string {
	b, _ := json.Marshal(c)
	return codePrefix + base64.RawURLEncoding.EncodeToString(b)
}

func ParseCode(s string) (Code, error) {
	var c Code
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, codePrefix) {
		return c, errors.New("that doesn't look like a Movie Night code (it should start with " + codePrefix + ")")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, codePrefix))
	if err != nil {
		return c, errors.New("the code is damaged; copy it again, all of it")
	}
	if err := json.Unmarshal(b, &c); err != nil || c.Device == "" || len(c.Folders) == 0 {
		return c, errors.New("the code is incomplete; copy it again, all of it")
	}
	return c, nil
}
