package secure

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// AuthorizationLevel indicates the access level for given API
type AuthorizationLevel int

const (
	NOAUTH AuthorizationLevel = iota
	READ
	READWRITE
	ADMIN
)

// LevelFromString finds the level corresponding
func LevelFromString(val string) (AuthorizationLevel, error) {
	switch val {
	case "read":
		return READ, nil
	case "readwrite":
		return READWRITE, nil
	case "admin":
		return ADMIN, nil
	default:
		return 0, fmt.Errorf("invalid authorization string")
	}
}

type Authorizer interface {
	// Authorize determines whether the user (email address) has the specified authorization level
	Authorize(user string, level AuthorizationLevel) bool

	// Level returns the authorization level for the user
	Level(user string) (AuthorizationLevel, error)
}

// FileAuthorize implements Authorizer using a JSON file for authorization
// that is dictionary of emails with values "read", "readwrite", "admin".
type FileAuthorize struct {
	fileName        string
	userPermissions map[string]string
}

func loadUsers(authFile string) (map[string]string, error) {
	fin, err := os.Open(authFile)
	defer fin.Close()
	if err != nil {
		err = fmt.Errorf("%s cannot be read", authFile)
		return nil, err
	}
	byteData, _ := ioutil.ReadAll(fin)
	var authlist map[string]string
	json.Unmarshal(byteData, &authlist)
	return authlist, nil
}

// NewFileAuthorizer creates a FileAuthorize object
func NewFileAuthorizer(authFile string) (FileAuthorize, error) {
	authList, err := loadUsers(authFile)
	if err != nil {
		return FileAuthorize{}, err
	}
	return FileAuthorize{authFile, authList}, nil
}

func (a FileAuthorize) Authorize(user string, level AuthorizationLevel) bool {
	perm, ok := a.userPermissions[user]
	found := ok
	if !ok {
		// re-check file in case user was recently added
		if authList2, err := loadUsers(a.fileName); err != nil {
			a.userPermissions = authList2
			perm, found = a.userPermissions[user]
		}
	}

	if found {
		// authorization level of user satisfies requeted level
		if templevel, err := LevelFromString(perm); templevel >= level && err == nil {
			return true
		}
	}
	return false
}

func (a FileAuthorize) Level(user string) (AuthorizationLevel, error) {
	perm, ok := a.userPermissions[user]
	found := ok
	if !ok {
		// re-check file in case user was recently added
		if authList2, err := loadUsers(a.fileName); err != nil {
			a.userPermissions = authList2
			perm, found = a.userPermissions[user]
		}
	}

	if found {
		// authorization level of user satisfies requeted level
		if level1, err := LevelFromString(perm); err == nil {
			return level1, nil
		} else {
			return 0, err
		}
	}
	return 0, fmt.Errorf("User not found")
}
