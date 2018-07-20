// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/errors"
)

func embedModelStructs(node ast.Node) (ast.Node, error) {
	rewriteFunc := func(n ast.Node) bool {
		i, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		s, ok := i.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Big hack, need a better way to discover is struct is from model or not
		if strings.Contains(i.Name.Name, "_") {
			return true
		}

		if !i.Name.IsExported() {
			return true
		}

		embedField := &ast.Field{
			Type: ast.NewIdent(fmt.Sprintf("model.%s", i.Name.Name)),
		}

		list := []*ast.Field{embedField}

		for _, f := range s.Fields.List {
			fieldName := ""
			if len(f.Names) != 0 {
				fieldName = f.Names[0].Name
			}

			if f.Names == nil {
				continue
			}

			if strings.Contains(fieldName, "XXX_") {
				list = append(list, f)
			}
		}

		s.Fields.List = list

		return true
	}

	ast.Inspect(node, rewriteFunc)

	return node, nil
}

func goList(dir string) ([]string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", dir)
	bytes, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "Can't list packages")
	}

	return strings.Fields(string(bytes)), nil
}

func parse(file string) (ast.Node, *token.FileSet, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	return node, fset, nil
}

func getModelProtoFile() string {
	dirs, err := goList("github.com/mattermost/mattermost-server/mmproto")
	if err != nil {
		panic(err)
	} else if len(dirs) != 1 {
		panic("More than one package dir, or no dirs!")
	}

	return path.Join(dirs[0], "model.pb.go")
}

func getModelWriteFile() string {
	dirs, err := goList("github.com/mattermost/mattermost-server/mmproto")
	if err != nil {
		panic(err)
	} else if len(dirs) != 1 {
		panic("More than one package dir, or no dirs!")
	}

	return path.Join(dirs[0], "model_generated.pb.go")
}

func main() {
	log.Println("Embedding model structs into mmproto structs")
	node, fset, err := parse(getModelProtoFile())
	if err != nil {
		log.Println("Unable to get parse model.pb.go: " + err.Error())
		return
	}

	rewrittenNode, _ := embedModelStructs(node)

	var buf bytes.Buffer
	err = format.Node(&buf, fset, rewrittenNode)
	if err != nil {
		log.Println(err)
		return
	}

	// Hack to add model import as adding it to the ast wasn't working
	strFile := string(buf.Bytes())
	strFile = strings.Replace(strFile, "import io \"io\"", "import io \"io\"\n\nimport \"github.com/mattermost/mattermost-server/model\"", 1)

	err = ioutil.WriteFile(getModelWriteFile(), []byte(strFile), 0755)
	if err != nil {
		log.Println(err)
	}
}
