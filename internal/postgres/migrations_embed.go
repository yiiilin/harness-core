package postgres

import (
	"embed"
	"path"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Migration struct {
	Version string
	Name    string
	SQL     string
}

func Migrations() []Migration {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		panic(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	migrations := make([]Migration, 0, len(names))
	for _, name := range names {
		body, err := migrationFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			panic(err)
		}
		version, label, _ := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		migrations = append(migrations, Migration{
			Version: version,
			Name:    label,
			SQL:     string(body),
		})
	}
	return migrations
}

func LatestMigrationVersion() string {
	migrations := Migrations()
	if len(migrations) == 0 {
		return ""
	}
	return migrations[len(migrations)-1].Version
}
