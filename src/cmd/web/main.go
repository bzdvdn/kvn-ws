// @sk-task kvn-web#T2.4: web entrypoint (AC-001)
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	"github.com/bzdvdn/kvn-ws/src/internal/webui"
)

func main() {
	port := flag.Int("port", 2311, "web UI port")
	openBrowser := flag.Bool("open-browser", true, "open browser on start")
	noBrowser := flag.Bool("no-browser", false, "suppress browser open (for daemon mode)")
	flag.Parse()

	srv, err := webui.New(*port)
	if err != nil {
		log.Fatalf("webui: %v", err)
	}

	if *openBrowser && !*noBrowser && isTerminal() {
		tryOpenBrowser(*port)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Printf("KVN Web UI started at http://127.0.0.1:%d", *port)
	if err := srv.Serve(ctx); err != nil {
		log.Printf("webui stopped: %v", err)
	}
}

func isTerminal() bool {
	stat, _ := os.Stdout.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func tryOpenBrowser(port int) {
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:gosec
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec
	default:
		return
	}
	cmd.Start() //nolint:errcheck
}
