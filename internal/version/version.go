package version

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Lines() string {
	return fmt.Sprintf("version=%s\ncommit=%s\ndate=%s\n", Version, Commit, Date)
}
