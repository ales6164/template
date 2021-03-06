package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"html/template"
	flag "github.com/ogier/pflag"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/html"
	"github.com/tdewolff/minify/js"
	"github.com/tdewolff/minify/css"
	"github.com/ales6164/pages"
	"path"
	"bytes"
)

type Compiler struct {
	content string
	pages   *pages.Pages

	i                   int
	opened              []Func
	predefinedFuncCalls []string
}

var settingsPath, flagLayout, flagPage, flagPath string
var serve bool

func init() {
	flag.StringVarP(&settingsPath, "settings", "s", "./settings.json", "Import settings.json")
	flag.StringVar(&flagLayout, "layout", "", "")
	flag.StringVar(&flagPage, "page", "", "")
	flag.StringVar(&flagPath, "path", "", "")
	flag.BoolVar(&serve, "serve", false, "")
}

func main() {
	flag.Parse()

	p, err := pages.New(&pages.Options{
		IsRendering:  true,
		JsonFilePath: settingsPath,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// serving or just compiling?
	if f := flag.Lookup("serve"); f != nil && f.Value.String() == "true" {
		buf := new(bytes.Buffer)
		err := p.Execute(buf, flagLayout, flagPage)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Println(buf.String())
	} else {
		var c = &Compiler{}
		c.init(p)
	}
}

func (c *Compiler) init(page *pages.Pages) {
	// create compiled path dir
	if _, err := os.Stat(path.Dir(page.Dist)); os.IsNotExist(err) {
		err = os.MkdirAll(path.Dir(page.Dist), os.ModePerm)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// create output js file
	output, err := os.Create(page.Dist)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer output.Close()

	// set minifier
	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/html", html.Minify)
	m.AddFunc("text/javascript", js.Minify)
	m.Add("text/html", &html.Minifier{
		KeepDefaultAttrVals:     true,
		KeepWhitespace:          false,
		KeepConditionalComments: true,
		KeepDocumentTags:        true,
		KeepEndTags:             true,
	})

	// compile templates
	for _, f := range page.TemplateFilePaths {
		tmpl, err := ioutil.ReadFile(f)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// remove comments
		// todo: fails if astrix (*) is a part of comment content
		compiledJS := regexp.MustCompile(`{{[^{]*(\/\*[^\*]*\*\/)[^}]*}}`).ReplaceAllString(string(tmpl), "")

		// minify html
		compiledJS, err = m.String("text/html", compiledJS)

		compiledJS = c.Compile(compiledJS)

		_, err = output.WriteString(compiledJS)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

// compile file
func (c *Compiler) Compile(text string) string {
	c.content = text
	c.content = regexp.MustCompile("\x60").ReplaceAllString(c.content, "\\\x60")
	c.content = regexp.MustCompile(`({{)[^}]+(}})`).ReplaceAllStringFunc(c.content, c.replace)

	return c.content
}

func (c *Compiler) replace(pipeline string) string {
	pipeline = strings.TrimLeft(pipeline, "{{")
	pipeline = strings.TrimRight(pipeline, "}}")
	pipeline = strings.TrimSpace(pipeline)

	// remove comments
	pipeline = regexp.MustCompile(`(\/\*).*(\*\/)`).ReplaceAllString(pipeline, "")

	// remove double spaces, newlines
	pipeline = regexp.MustCompile(`(\s|\v|\n)+`).ReplaceAllString(pipeline, " ")

	return c.renderPipeline(pipeline)
}

// currently doesn't support real pipelines
// with if eq "sth" "othr"
func (c *Compiler) renderPipeline(pipeline string) string {
	commands := strings.Split(pipeline, " ")
	return c.renderCommand(commands)
}

func (c *Compiler) renderCommand(commands []string) string {
	var render string

	cmd := commands[0]
	//fmt.Println("command", cmd)

	switch cmd {
	case "define":
		c.putFunc(FuncDefine(c, c.evalInner(commands[1:])))
		render = c.opened[c.i].start()
	case "with":
		c.putFunc(FuncWith(c.evalInner(commands[1:])))
		render = c.opened[c.i].start()
	case "template":
		fun := FuncTemplate(c.evalInner(commands[1:]))
		render = fun.start()
	case "range":
		c.putFunc(FuncRange(c.evalInner(commands[1:])))
		render = c.opened[c.i].start()
	case "end":
		render = c.endFunc()
		/*fmt.Println("ending with", render, len(s.opened))*/
	default:
		if strings.HasPrefix(cmd, ".") {
			if len(cmd) == 1 {
				render = "${$$}"
			} else {
				render = "${$$" + cmd + "}"
			}
		} else if strings.HasPrefix(cmd, `"`) && strings.HasSuffix(cmd, `"`) {
			render = "${" + cmd + "}"
		} else if strings.HasPrefix(cmd, `$`) {
			render = "${" + cmd + "}"
		}
	}

	return render
}

func (c *Compiler) putFunc(f Func) {
	c.opened = append(c.opened, f)
	c.i = len(c.opened) - 1
}

func (c *Compiler) endFunc() string {
	end := c.opened[c.i].end()
	c.opened = c.opened[:len(c.opened)-1]
	c.i = len(c.opened) - 1
	return end
}

func (c *Compiler) evalInner(inner []string) string {
	var out []string
	for i := len(inner) - 1; i >= 0; i-- {
		tmp := ""
		cmd := inner[i]
		switch cmd {
		/*case "eq":
			// call to some tmp=func(out)string*/
		/*case "fetch":
			c.predefinedFuncCalls = append(c.predefinedFuncCalls, `['fetch', '`+c.pages.API+strings.Join(out, "/")+`']`)
			out = []string{"$$$[" + strconv.Itoa(len(c.predefinedFuncCalls)-1) + "]"}*/
		default:
			if strings.HasPrefix(cmd, ".") {
				if len(cmd) == 1 {
					tmp = "$$"
				} else {
					tmp = "$$" + cmd
				}
			} else if strings.HasPrefix(cmd, `"`) && strings.HasSuffix(cmd, `"`) {
				tmp = cmd
			} else {
				tmp = cmd
			}
		}

		// prepend
		out = append([]string{tmp}, out...)
	}
	return strings.Join(out, " ")
}

type Func interface {
	start() string
	orElse() string
	end() string
}

type funcDefine struct {
	c    *Compiler
	name string
}

/* DEFINE */

func FuncDefine(c *Compiler, name string) *funcDefine {
	return &funcDefine{
		c:    c,
		name: name,
	}
}

func (f *funcDefine) start() string {
	return "customComponents.define(" + f.name + ",($,$$$)=>{let $$=$;return`"
}

func (f *funcDefine) orElse() string {
	return ""
}

func (f *funcDefine) end() string {
	var predefFuns string
	if len(f.c.predefinedFuncCalls) > 0 {
		predefFuns = ",[" + strings.Join(f.c.predefinedFuncCalls, ",") + "]"

	}
	return "`}" + predefFuns + ");"
}

/* WITH */

type funcWith struct {
	obj     string
	hasElse bool
}

func FuncWith(obj string) *funcWith {
	return &funcWith{
		obj: obj,
	}
}

func (f *funcWith) start() string {
	return "${" + f.obj + "?(($$)=>{return`"
}

func (f *funcWith) orElse() string {
	f.hasElse = true
	return "`})(" + f.obj + "):`"
}

func (f *funcWith) end() string {
	if f.hasElse {
		return "`}"
	}
	return "`})(" + f.obj + "):``}"
}

/* TEMPLATE */

type funcTemplate struct {
	context string
	name    string
}

func FuncTemplate(nameContext string) *funcTemplate {
	var name, context string
	nc := strings.Split(nameContext, " ")
	name = nc[0]
	if len(nc) > 1 {
		context = nc[1]
	}

	return &funcTemplate{
		name:    name,
		context: context,
	}
}

func (f *funcTemplate) start() string {
	var ctx string
	if len(f.context) > 0 {
		ctx = "," + f.context
	}
	return "${customComponents.render(" + f.name + ctx + ")}"
}

func (f *funcTemplate) orElse() string {
	return ""
}

func (f *funcTemplate) end() string {
	return ""
}

/* RANGE */

type funcRange struct {
	obj     string
	i       string
	val     string
	hasElse bool
}

func FuncRange(obj string) *funcRange {
	var i string
	var val string
	fmt.Println("RANGE", obj)
	obj1 := strings.Split(obj, ":=")
	if len(obj1) == 2 {
		obj = strings.TrimSpace(obj1[1])
		obj2 := strings.Split(obj1[0], ",")
		if len(obj2) == 2 {
			i = strings.TrimSpace(obj2[0])
			val = strings.TrimSpace(obj2[1])
		} else {
			val = strings.TrimSpace(obj2[0])
		}
	}
	return &funcRange{
		obj: obj,
		i:   i,
		val: val,
	}
}

func (f *funcRange) start() string {
	var vars string
	if len(f.val) > 0 {
		vars += f.val
		if len(f.i) > 0 {
			vars += "," + f.i
		}
	}

	return "${" + f.obj + " && " + f.obj + ".length>0 ? " + f.obj + ".map((" + vars + ")=>`"
}

func (f *funcRange) orElse() string {
	f.hasElse = true
	return "`).join(''):`"
}

func (f *funcRange) end() string {
	if f.hasElse {
		return "`}"
	}
	return "`).join(''):``}"
}

type Context struct {
	PageTemplate string
	Data         map[string]interface{}
}

type Settings struct {
	basePath  string
	path      string
	templates *template.Template

	Templates    []string  `json:"templates"`
	CompiledPath string    `json:"compiled_path"`
	Routers      []*Router `json:"routers"`
	Layout       string    `json:"layout"`
}

type Router struct {
	Name   string `json:"name"`
	Layout string `json:"layout"`
	Handle map[string]*Route
}

type Route struct {
	Layout string `json:"layout"`
	Page   string `json:"page"`
}
