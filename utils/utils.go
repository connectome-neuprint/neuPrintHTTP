package utils

import (
	"strings"
)

// CheckSubsetVersion is true if versioncheck is a subset of version
func CheckSubsetVersion(versioncheck, version string) bool {
	versionsplit := strings.Split(versioncheck, ".")
	curr_versionsplit := strings.Split(version, ".")

	for idx, part := range versionsplit {
		if part != "" {
			if idx >= len(curr_versionsplit) || part != curr_versionsplit[idx] {
				return false
			}
		}
	}
	/*version, _ := strconv.Atoi(versionsplit[0])
	curr_version, _ := strconv.Atoi(curr_versionsplit[0])
	if version != curr_version {
		return c.HTML(http.StatusBadRequest, "Incompatible API version")
	}*/

	return true
}
