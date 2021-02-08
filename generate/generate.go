package generate

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fioncat/go-gendb/build"
	"github.com/fioncat/go-gendb/compile/mediate"
	"github.com/fioncat/go-gendb/compile/parse"
	"github.com/fioncat/go-gendb/compile/scan/sgo"
	"github.com/fioncat/go-gendb/compile/token"
	"github.com/fioncat/go-gendb/database/rdb"
	"github.com/fioncat/go-gendb/generate/coder"
	"github.com/fioncat/go-gendb/generate/internal/gorm"
	"github.com/fioncat/go-gendb/generate/internal/gsql"
	"github.com/fioncat/go-gendb/generate/internal/gstruct"
	"github.com/fioncat/go-gendb/misc/errors"
	"github.com/fioncat/go-gendb/misc/log"
	"github.com/fioncat/go-gendb/misc/trace"
	"github.com/fioncat/go-gendb/misc/wpool"
)

// Each generator must implement the four methods of this
// interface before it can be used for code generation.
type generator interface {
	// Name returns the name of the generator, which must
	// be unique, and it corresponds to mediate.Result.Type.
	Name() string

	// ConfType returns the potential configuration data
	// type of the generator. Need to return a pointer,
	// in the form "(*Type)(nil)". If the generator does
	// not support configuration, just return nil.
	// If the user passes a parameter to the generator,
	// it will use reflection to create the structure type
	// data returned by the function, and assign the value
	// with the parameter passed in by the user, and finally
	// it will be passed to the confv parameter of the Do
	// method.
	ConfType() interface{}

	// DefaultConf returns to the default configuration.
	// If the user does not pass generator parameters,
	// the return value of this method will be used as
	// the Do.confv parameter.
	DefaultConf() interface{}

	// Do performs the code generation process. "c" is
	// used to generate code, "result" is the intermediate
	// result of compilation, its Type corresponds to the
	// name of the current generator, "confv" is the generator
	// configuration passed in by the user (or the default),
	// if the generator Configuration is not supported, this
	// parameter can be ignored.
	Do(c *coder.Coder, result mediate.Result, confv interface{}) error
}

// Arg is the command line parameter structure passed in
// to execute code generation.
// For details on the fields, see the command line help
// documentation.
type Arg struct {
	Cache    bool   `flag:"cache"`
	CacheTTL string `flag:"cache-ttl"`
	Log      bool   `flag:"log"`
	LogPath  string `flag:"log-path"`
	ConfPath string `flag:"conf-path"`

	Conn   string `flag:"conn"`
	DbType string `flag:"db-type" default:"mysql"`

	Path string `arg:"path"`
}

// prepare code generation
func prepare(arg *Arg) error {
	if arg.Log {
		log.Init(true, arg.LogPath)
	}
	if arg.Cache {
		rdb.ENABLE_TABLE_CACHE = true
	}
	if arg.CacheTTL != "" {
		// parse and set table cache TimeToLive
		ttl, err := time.ParseDuration(arg.CacheTTL)
		if err != nil {
			return fmt.Errorf("ttl bad format: %s", arg.CacheTTL)
		}
		rdb.TABLE_CACHE_TTL = ttl
	}

	if arg.Conn != "" {
		// need to connect database
		err := rdb.Init(arg.Conn, arg.DbType)
		if err != nil {
			return errors.Trace("init database", err)
		}
	}
	return nil
}

// One performs code generation on a file, and the entire
// process includes compilation and code generation.
func One(arg *Arg) error {
	err := prepare(arg)
	if err != nil {
		return err
	}
	data, err := ioutil.ReadFile(arg.Path)
	if err != nil {
		return err
	}
	var confData []byte
	if arg.ConfPath != "" {
		confData, err = ioutil.ReadFile(arg.ConfPath)
		if err != nil {
			return err
		}
	}
	return one(arg.Path, data, confData)
}

// implement of compilation and generation.
func one(path string, data, confData []byte) error {
	tt := trace.NewTimer("gen:" + path)
	defer tt.Trace()

	tt.Start("scan")
	sr, err := sgo.Do(path, string(data))
	if err != nil {
		return err
	}

	tt.Start("parse")
	results, err := parse.Do(sr)
	if err != nil {
		return err
	}

	tt.Start("generate")
	dir := filepath.Dir(path)
	return do(path, dir, sr.Package, confData, results)
}

// Batch will concurrently scan all go files in the specified
// directory, and if they have been marked to generate code,
// they will perform code generation; skip unmarked files.
// Because all go files in the directory are scanned, this
// function will take up more IO resources.
func Batch(arg *Arg) error {
	err := prepare(arg)
	if err != nil {
		return err
	}
	var confData []byte
	if arg.ConfPath != "" {
		confData, err = ioutil.ReadFile(arg.ConfPath)
		if err != nil {
			return err
		}
	}
	return batch(arg.Path, confData)
}

// implement of Batch
func batch(root string, confData []byte) error {
	var paths []string
	tt := trace.NewTimer("batch:" + root)
	defer tt.Trace()

	tt.Start("fetch")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		filename := filepath.Base(path)
		if strings.HasSuffix(filename, ".go") {
			paths = append(paths, path)
		}

		return nil
	})

	if err != nil {
		return errors.Trace("fetch files", err)
	}

	if len(paths) == 0 {
		return fmt.Errorf("no file to generate")
	}

	tt.Start("generate")
	wp := wpool.New(build.N_WORKERS, len(paths))
	for _, path := range paths {
		path := path
		wp.Submit(func() error {
			return genWorker(path, confData)
		})
	}

	if err := wp.Wait(); err != nil {
		return err
	}

	return nil
}

func genWorker(path string, confData []byte) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	// scan tags
	hasTag := false
	for _, line := range lines {
		if token.TAG_PREFIX.Prefix(line) {
			hasTag = true
			break
		}
		if token.GO_PACKAGE.Prefix(line) {
			break
		}
	}
	if !hasTag {
		// no tag, directly return.
		return nil
	}

	// has tag, process code generation.
	return one(path, data, confData)
}

// Preloading of generators
var (
	generators   = make(map[string]generator)
	genConfTypes = make(map[string]reflect.Type)

	genConfs sync.Map
)

func init() {
	generators["db-oper"] = &gsql.Generator{}
	generators["sql-orm"] = &gorm.Generator{}
	generators["struct"] = &gstruct.Generator{}

	for key, gen := range generators {
		t := gen.ConfType()
		if t == nil {
			continue
		}
		genConfTypes[key] = reflect.TypeOf(gen.ConfType()).Elem()
	}
}

// The lowest level function that performs code generation.
// Specific steps:
//   1. Find the generator according to the result, if not
//      found, return an error
//   2. If the generator has a set configuration structure
//      and the user passes in the configuration, the user's
//      configuration is parsed and converted into the
//      generator's configuration data.
//   3. If the generator has a configuration configuration
//      structure, but the user does not pass in the configuration,
//      use the default configuration of the generator.
//   4. Initialize coder.Coder, execute the Do method of the
//      generator, and generate code.
//   5. Write the generated code to disk.
//
// Once an error occurs in the above process, the files that have
// been generated by the current do will be deleted.
func do(path, dir, pkg string, confData []byte, results []mediate.Result) error {
	paths := make([]string, 0, len(results))
	for _, result := range results {
		generator := generators[result.Type()]
		if generator == nil {
			onRemove(paths)
			return fmt.Errorf("can not find "+
				"generator: %s", result.Type())
		}

		c := new(coder.Coder)
		c.Source = path
		c.Pkg = pkg

		confv, _ := genConfs.Load(result.Type())
		if confv == nil && len(confData) > 0 {
			conft, ok := genConfTypes[result.Type()]
			if ok {
				confv = reflect.New(conft).Interface()
				err := json.Unmarshal(confData, confv)
				if err != nil {
					onRemove(paths)
					return errors.Trace("unmarshal conf data", err)
				}
				genConfs.Store(result.Type(), confv)
			}
		}

		if confv == nil {
			confv = generator.DefaultConf()
		}

		err := generator.Do(c, result, confv)
		if err != nil {
			onRemove(paths)
			return errors.Trace("generate", err)
		}

		tarName := fmt.Sprintf("zz_generated_%s.go", result.Key())
		tarPath := filepath.Join(dir, tarName)
		err = c.Write(tarPath)
		if err != nil {
			onRemove(paths)
			return errors.Trace("write target file", err)
		}
		paths = append(paths, tarPath)
	}
	return nil
}

func onRemove(paths []string) {
	for _, path := range paths {
		os.Remove(path)
	}
}
