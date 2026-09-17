package discovery

import (
	"net/http"
	"path/filepath"
	"strings"
)

type processInfo struct {
	name string
	args []string
	cwd  string
}

var ignoredProcessPrefixes = []string{
	"ControlCenter",
	"Code Helper",
	"Notion",
	"Postman",
	"cloudflared",
	"idea",
	"goland",
	"phpstorm",
	"webstorm",
	"pycharm",
}

var frameworkByCommand = map[string]string{
	"next":               "Next.js",
	"nuxt":               "Nuxt",
	"vite":               "Vite",
	"astro":              "Astro",
	"webpack":            "webpack",
	"webpack-dev-server": "webpack",
	"artisan":            "Laravel",
	"manage.py":          "Django",
	"rails":              "Rails",
	"uvicorn":            "Python",
	"gunicorn":           "Python",
	"flask":              "Flask",
}

type headerRule struct {
	keyword   string
	framework string
}

var headerRules = []headerRule{
	{"next.js", "Next.js"},
	{"express", "Express"},
	{"php", "PHP"},
	{"nginx", "nginx"},
	{"apache", "Apache"},
	{"caddy", "Caddy"},
	{"werkzeug", "Flask"},
}

func isIgnored(processName string) bool {
	for _, prefix := range ignoredProcessPrefixes {
		if strings.HasPrefix(processName, prefix) {
			return true
		}
	}
	return false
}

func inferName(p processInfo, home string) string {
	if strings.HasPrefix(p.cwd, home+string(filepath.Separator)) {
		return filepath.Base(p.cwd)
	}
	return p.name
}

func inferFramework(p processInfo, header http.Header) string {
	for _, arg := range p.args {
		if framework, ok := frameworkByCommand[filepath.Base(arg)]; ok {
			return framework
		}
	}
	banner := strings.ToLower(header.Get("X-Powered-By") + " " + header.Get("Server"))
	for _, rule := range headerRules {
		if strings.Contains(banner, rule.keyword) {
			return rule.framework
		}
	}
	return ""
}
