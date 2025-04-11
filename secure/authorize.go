package secure

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
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
	case "readonly":
		return READ, nil
	case "readwrite":
		return READWRITE, nil
	case "admin":
		return ADMIN, nil
	case "noauth":
		return NOAUTH, nil
	default:
		return 0, fmt.Errorf("invalid authorization string")
	}
}

// LevelFromString finds the level corresponding
func StringFromLevel(level AuthorizationLevel) (string, error) {
	switch level {
	case READ:
		return "readonly", nil
	case READWRITE:
		return "readwrite", nil
	case ADMIN:
		return "admin", nil
	case NOAUTH:
		return "noauth", nil
	default:
		return "", fmt.Errorf("invalid authorization string")
	}
}

type Authorizer interface {
	// Authorize determines whether the user (email address) has the specified authorization level
	Authorize(user string, level AuthorizationLevel) bool

	// Level returns the authorization level for the user
	Level(user string) (AuthorizationLevel, error)
}

// DatastoreAuthorize implements Authorizer using the appdata-store cloud function http interface.
type DatastoreAuthorize struct {
	httpAddr        string
	token           string
	userPermissions map[string]string
}

// NewFileAuthorizer creates a FileAuthorize object
func NewDatastoreAuthorizer(httpAddr string, token string) (DatastoreAuthorize, error) {
	authList, err := loadDatastoreUsers(httpAddr, token)
	if err != nil {
		return DatastoreAuthorize{}, err
	}

	return DatastoreAuthorize{httpAddr, token, authList}, nil
}

func loadDatastoreUsers(httpAddr, token string) (map[string]string, error) {
	datastoreClient := http.Client{
		Timeout: time.Second * 60,
	}

	req, err := http.NewRequest(http.MethodGet, httpAddr+"/users", nil)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := datastoreClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}

	var authlist map[string]string
	jsonErr := json.Unmarshal(body, &authlist)
	if jsonErr != nil {
		return nil, fmt.Errorf("could not retrieve authorization information")
	}

	return authlist, nil
}

func (a DatastoreAuthorize) Authorize(user string, level AuthorizationLevel) bool {
	// if the authorization level is disabled, return true
	if level == NOAUTH {
		return true
	}
	perm, ok := a.userPermissions[user]
	found := ok
	if !ok {
		// re-check file in case user was recently added
		if authList2, err := loadDatastoreUsers(a.httpAddr, a.token); err == nil {
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

func (a DatastoreAuthorize) Level(user string) (AuthorizationLevel, error) {
	perm, ok := a.userPermissions[user]
	found := ok
	if !ok {
		// re-check file in case user was recently added
		if authList2, err := loadDatastoreUsers(a.httpAddr, a.token); err == nil {
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

	return NOAUTH, nil
}

// FileAuthorize implements Authorizer using a JSON file for authorization
// that is dictionary of emails with values "readonly", "readwrite", "admin".
type FileAuthorize struct {
	fileName        string
	userPermissions map[string]string
}

func loadUsers(authFile string) (map[string]string, error) {
	fin, err := os.Open(authFile)
	if err != nil {
		err = fmt.Errorf("%s cannot be read", authFile)
		return nil, err
	}
	defer fin.Close()
	byteData, _ := io.ReadAll(fin)
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
	// if the authorization level is disabled, return true
	if level == NOAUTH {
		return true
	}
	perm, ok := a.userPermissions[user]
	found := ok
	if !ok {
		// re-check file in case user was recently added
		if authList2, err := loadUsers(a.fileName); err == nil {
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
		if authList2, err := loadUsers(a.fileName); err == nil {
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

	return NOAUTH, nil
}
