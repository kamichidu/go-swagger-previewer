package main

import (
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

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
		log.Infof("%s %s %s", req.Method, req.RequestURI, req.Proto)
		next.ServeHTTP(w, req)
	})
}

func replaceContent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, req)
		body := rec.Body.String()
		if strings.HasPrefix(http.DetectContentType([]byte(body)), "text/html") {
			body = strings.Replace(
				body,
				`url: "http://petstore.swagger.io/v2/swagger.json",`,
				``,
				-1)
			body = strings.Replace(
				body,
				`dom_id: '#swagger-ui',`,
				strings.Join([]string{
					`dom_id: '#swagger-ui',`,
					`configUrl: '/.swagger-config.yaml',`,
				}, "\n"),
				-1)
		}
		for k, vals := range rec.HeaderMap {
			if k == "Content-Length" {
				continue
			}
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(rec.Code)
		w.Write([]byte(body))
	})
}

type fallbackFS struct {
	Dirs []string
}

func (fs *fallbackFS) Open(name string) (http.File, error) {
	for _, dir := range fs.Dirs {
		f, err := os.Open(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		return f, nil
	}
	return nil, os.ErrNotExist
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, req)
	})
}

func swaggerConfig(dir string) http.Handler {
	var urls []map[string]string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		switch filepath.Ext(info.Name()) {
		case ".json", ".yml", ".yaml":
			relname, err := filepath.Rel(dir, p)
			if err != nil {
				panic(err)
			}
			relname = path.Join("/", path.Clean(filepath.ToSlash(relname)))
			urls = append(urls, map[string]string{
				"name": relname,
				"url":  relname,
			})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cfg := map[string]interface{}{}
		var def string
		if referer, err := url.Parse(req.Referer()); err == nil {
			def = referer.Query().Get("url")
		}
		if len(urls) > 0 {
			if def != "" {
				cfg["urls.primaryName"] = def
			}
			cfg["urls"] = urls
		} else {
			cfg["url"] = def
		}
		cfg["validatorUrl"] = "/validate"
		content, err := yaml.Marshal(cfg)
		if err != nil {
			panic(err)
		}
		w.Write(content)
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
		fmt.Fprintln(errOut, "go-swagger-previewer [options] {path/to/dir/contains-swagger.yml}")
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "available options:")
		flag.PrintDefaults()
	}
	flag.StringVar(&addr, "l", ":8080", "listen `ADDR`")
	flag.BoolVar(&verbose, "vv", false, "verbose logging")
	flag.BoolVar(&update, "u", false, "update swagger-ui")
	flag.Parse()

	rootDir := flag.Arg(0)
	if rootDir == "" {
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
	mux.Handle("/", noCache(replaceContent(http.FileServer(&fallbackFS{
		Dirs: []string{
			swaggerUIDistDir,
			rootDir,
		},
	}))))
	mux.Handle("/.swagger-config.yaml", noCache(swaggerConfig(rootDir)))
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
