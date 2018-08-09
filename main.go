package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	log "github.com/sirupsen/logrus"
)

func cacheDir() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		var homeDir string
		if runtime.GOOS == "windows" {
			homeDir = os.Getenv("AppData")
		} else {
			homeDir = os.Getenv("HOME")
		}
		if homeDir == "" {
			log.Panic("unable to get user's home dir")
		}
		dir = filepath.Join(homeDir, ".cache")
	}
	return filepath.Join(dir, "go-swagger-previewer")
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		log.Infof("%s", req.RequestURI)
		next.ServeHTTP(w, req)
	})
}

func urlQueryParam(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if q := req.URL.Query(); q.Get("url") == "" {
			log.Debugf("virtual host: %s", req.Host)
			log.Debugf("request url: %s", req.URL)
			u := *req.URL
			if u.Scheme == "" {
				u.Scheme = "http"
			}
			if u.Host == "" {
				u.Host = req.Host
			}
			u.Path = "/swagger.yml"
			q.Set("url", u.String())
			req.URL.RawQuery = q.Encode()
			http.Redirect(w, req, req.URL.String(), http.StatusFound)
		} else {
			next.ServeHTTP(w, req)
		}
	})
}

func run(in io.Reader, out io.Writer, errOut io.Writer, args []string) int {
	if f, ok := errOut.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		log.SetOutput(colorable.NewColorable(f))
		log.SetFormatter(&log.TextFormatter{
			ForceColors:   true,
			FullTimestamp: true,
		})
	} else {
		log.SetOutput(errOut)
		log.SetFormatter(&log.TextFormatter{})
	}

	var (
		addr    string
		verbose bool
		update  bool
	)
	flag.CommandLine.SetOutput(errOut)
	flag.Usage = func() {
		fmt.Fprintln(errOut, "go-swagger-previewer [options] {path/to/swagger.yml}")
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "available options:")
		flag.PrintDefaults()
	}
	flag.StringVar(&addr, "l", ":8080", "listen `ADDR`")
	flag.BoolVar(&verbose, "vv", false, "verbose logging")
	flag.BoolVar(&update, "u", false, "update swagger-ui")
	flag.Parse()

	if filename := flag.Arg(0); filename == "" {
		flag.Usage()
		return 128
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	swaggerUIDistDir := filepath.Join(cacheDir(), "dist")
	needUpdate := update
	if _, err := os.Stat(swaggerUIDistDir); err != nil {
		needUpdate = true
	}
	if needUpdate {
		log.Info("updating swagger-ui")
		if err := downloadSwaggerUI(cacheDir()); err != nil {
			log.Fatalf("unable to update swagger-ui: %s", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", urlQueryParam(http.FileServer(http.Dir(swaggerUIDistDir))))
	mux.HandleFunc("/swagger.yml", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, flag.Arg(0))
	})
	mux.Handle("/validate", validate(flag.Arg(0)))

	srv := &http.Server{
		Addr:    addr,
		Handler: requestLogger(mux),
	}
	defer srv.Close()

	log.Infof("listening %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Error(err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr, os.Args))
}
