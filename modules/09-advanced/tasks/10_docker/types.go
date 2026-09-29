package dockerapp

import "fmt"

// Info — информация о сборке, отдаётся на GET /version.
type Info struct {
	Version   string `json:"version"`    // из -ldflags "-X main.version=..." или "dev"
	Commit    string `json:"commit"`     // из -ldflags, иначе vcs.revision, иначе "unknown"
	Dirty     bool   `json:"dirty"`      // vcs.modified == "true"
	GoVersion string `json:"go_version"` // версия компилятора
	Module    string `json:"module"`     // путь главного модуля
}

func (i Info) String() string {
	return fmt.Sprintf("%s (%s, %s)", i.Version, i.Commit, i.GoVersion)
}
