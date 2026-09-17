package discovery

import (
	"net/http"
	"testing"
)

func TestIsIgnored(t *testing.T) {
	tests := []struct {
		name    string
		process string
		want    bool
	}{
		{"airplay receiver", "ControlCenter", true},
		{"vs code helper variant", "Code Helper (Plugin)", true},
		{"jetbrains ide", "phpstorm", true},
		{"dev server runtime", "node", false},
		{"unknown process", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIgnored(tt.process); got != tt.want {
				t.Errorf("isIgnored(%q) = %v, want %v", tt.process, got, tt.want)
			}
		})
	}
}

func TestInferName(t *testing.T) {
	const home = "/Users/dev"
	tests := []struct {
		name    string
		process processInfo
		want    string
	}{
		{"project directory under home", processInfo{name: "node", cwd: "/Users/dev/code/dashboard"}, "dashboard"},
		{"home itself falls back to process", processInfo{name: "node", cwd: "/Users/dev"}, "node"},
		{"daemon directory falls back to process", processInfo{name: "nginx", cwd: "/opt/homebrew"}, "nginx"},
		{"sibling of home is not under home", processInfo{name: "node", cwd: "/Users/developer/app"}, "node"},
		{"unknown process stays empty", processInfo{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferName(tt.process, home); got != tt.want {
				t.Errorf("inferName(%+v) = %q, want %q", tt.process, got, tt.want)
			}
		})
	}
}

func TestInferFramework(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		header http.Header
		want   string
	}{
		{"command in node_modules bin", []string{"node", "/app/node_modules/.bin/vite", "--port", "5173"}, nil, "Vite"},
		{"artisan serve", []string{"php", "artisan", "serve"}, nil, "Laravel"},
		{"command wins over header", []string{"node", "/app/node_modules/.bin/next", "dev"}, http.Header{"X-Powered-By": {"Express"}}, "Next.js"},
		{"powered-by header", []string{"node", "server.js"}, http.Header{"X-Powered-By": {"Express"}}, "Express"},
		{"server header ignores case", []string{"nginx"}, http.Header{"Server": {"nginx/1.27.5"}}, "nginx"},
		{"unknown", []string{"./api"}, http.Header{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferFramework(processInfo{args: tt.args}, tt.header); got != tt.want {
				t.Errorf("inferFramework(%v, %v) = %q, want %q", tt.args, tt.header, got, tt.want)
			}
		})
	}
}
