package commands

import (
	"errors"
	"fmt"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
	"time"

	"github.com/TNTworks/flogo-cli/common"
	"github.com/TNTworks/flogo-cli/util"
)

const (
	fileImportsGo = "imports.go"

	UpdateOptRebuild = iota
	UpdateOptAdd
	UpdateOptRemove
	UpdateOptUpdate
)

func UpdateCLI(pluginPkg string, updateOption int) error {

	exPath, err := os.Executable()
	if err != nil {
		return err
	}

	pluginSet := make(map[string]struct{})

	//add installed plugins
	installedPlugins := common.GetPluginPkgs()
	for _, aPluginPkg := range installedPlugins {
		pluginSet[aPluginPkg] = struct{}{}
		fmt.Println(aPluginPkg)
	}

	if updateOption == UpdateOptAdd {
		// add new plugin
		pluginSet[pluginPkg] = struct{}{}
	} else if updateOption == UpdateOptRemove {
		delete(pluginSet, pluginPkg)
	}

	path, ver, err := util.GetCLIInfo()

	tmpDir := os.TempDir()
	basePath := filepath.Join(tmpDir, "cli")
	cliCmdPath := filepath.Join(basePath, "cmd", "flogo")

	err = os.RemoveAll(basePath)
	if err != nil {
		fmt.Println("del err:", err)
	}

	err = util.Copy(path, basePath, false)
	if err != nil {
		return err
	}

	err = util.CreateVersionFile(cliCmdPath, ver)
	if err != nil {
		return err
	}

	err = createPluginListFile(basePath, pluginSet)
	if err != nil {
		return err
	}

	for plugin := range pluginSet {
		_, err := addPlugin(cliCmdPath, plugin)
		if err != nil {
			fmt.Println("error:", err)
		}
	}

	if updateOption == UpdateOptUpdate {
		err = util.ExecCmd(exec.Command("go", "get", "-u", pluginPkg), cliCmdPath)
		if err != nil {
			return err
		}
	}

	err = util.ExecCmd(exec.Command("go", "mod", "download"), basePath)
	if err != nil {
		return err
	}

	err = util.ExecCmd(exec.Command("go", "build"), cliCmdPath)
	if err != nil {
		//fmt.Fprintf(os.Stderr, "Error: %v\n", osErr)
		return err
	}

	cliExe := "flogo"
	if runtime.GOOS == "windows" || os.Getenv("GOOS") == "windows" {
		cliExe = cliExe + ".exe"
	}

	err = util.Copy(filepath.Join(cliCmdPath, cliExe), exPath, false)
	if err != nil {
		//fmt.Fprintf(os.Stderr, "Error: %v\n", osErr)
		return err
	}

	return nil
}

func addPlugin(cliCmdPath, pluginPkg string) (bool, error) {

	err := util.ExecCmd(exec.Command("go", "get", pluginPkg), cliCmdPath)
	if err != nil {
		return false, err
	}

	added, err := addPluginImport(cliCmdPath, pluginPkg)
	if err != nil {
		return added, err
	}

	return added, nil
}

func addPluginImport(cliCmdPath, pkg string) (bool, error) {
	importsFile := filepath.Join(cliCmdPath, fileImportsGo)

	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, importsFile, nil, parser.ImportsOnly)

	if file.Imports == nil {
		return false, errors.New("no imports found")
	}

	successful := util.AddImport(fset, file, pkg)

	if successful {
		f, err := os.Create(importsFile)
		if err != nil {
			return false, err
		}
		defer f.Close()
		if err := printer.Fprint(f, fset, file); err != nil {
			return false, err
		}
	}

	return successful, nil
}

func createPluginListFile(basePath string, plugins map[string]struct{}) error {

	f, err := os.Create(filepath.Join(basePath, "common", "pluginlist.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	err = pluginListTemplate.Execute(f, struct {
		Timestamp  time.Time
		PluginList map[string]struct{}
	}{
		Timestamp:  time.Now(),
		PluginList: plugins,
	})

	return err
}

var pluginListTemplate = template.Must(template.New("").Parse(`// Code generated by go generate; DO NOT EDIT.
// {{ .Timestamp }}
package common

func init() {

	{{range $k, $v := .PluginList}}
	pluginPkgs = append(pluginPkgs, "{{$k}}")
	{{end}}
}
`))
