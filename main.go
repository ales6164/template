package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"html/template"
)

type Site struct {
	content string

	i      int
	opened []Func
}

func main() {
	// parse template for real to check for any errors
	text, err := ioutil.ReadFile("templ.html")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	t := template.New("")
	t, err = t.Parse(string(text))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var s = &Site{}
	// remove comments
	// fails if astrix (*) is a part of comment content
	html := regexp.MustCompile(`{{[^{]*(\/\*[^\*]*\*\/)[^}]*}}`).ReplaceAllString(string(text), "")

	fmt.Print(s.Compile(html))
}

// compile file
func (s *Site) Compile(text string) string {
	s.content = text
	s.content = regexp.MustCompile(`({{)[^}]+(}})`).ReplaceAllStringFunc(s.content, s.replace)
	return s.content
}

func (s *Site) replace(pipeline string) string {
	pipeline = strings.TrimLeft(pipeline, "{{")
	pipeline = strings.TrimRight(pipeline, "}}")
	pipeline = strings.TrimSpace(pipeline)

	// remove comments
	pipeline = regexp.MustCompile(`(\/\*).*(\*\/)`).ReplaceAllString(pipeline, "")

	// remove double spaces, newlines
	pipeline = regexp.MustCompile(`(\s|\v|\n)+`).ReplaceAllString(pipeline, " ")

	return s.renderPipeline(pipeline)
}

// currently doesn't support real pipelines
// with if eq "sth" "othr"
func (s *Site) renderPipeline(pipeline string) string {
	commands := strings.Split(pipeline, " ")
	return s.renderCommand(commands)
}

func (s *Site) renderCommand(commands []string) string {
	var render string

	cmd := commands[0]
	//fmt.Println("command", cmd)

	switch cmd {
	case "define":
		s.putFunc(FuncDefine(evalInner(commands[1:])))
		render = s.opened[s.i].start()
	case "with":
		s.putFunc(FuncWith(evalInner(commands[1:])))
		render = s.opened[s.i].start()
	case "range":
		s.putFunc(FuncRange(evalInner(commands[1:])))
		render = s.opened[s.i].start()
	case "end":
		render = s.endFunc()
		fmt.Println("ending with", render, len(s.opened))
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

func (s *Site) putFunc(f Func) {
	s.opened = append(s.opened, f)
	s.i = len(s.opened) - 1
}

func (s *Site) endFunc() string {
	end := s.opened[s.i].end()
	s.opened = s.opened[:len(s.opened)-1]
	s.i = len(s.opened) - 1
	return end
}

func evalInner(inner []string) string {
	var out []string
	for i := len(inner) - 1; i >= 0; i-- {
		tmp := ""
		cmd := inner[i]
		switch cmd {
		/*case "eq":
			// call to some tmp=func(out)string*/
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
	name string
}

/* DEFINE */

func FuncDefine(name string) *funcDefine {
	return &funcDefine{
		name: name,
	}
}

func (f *funcDefine) start() string {
	return "define(" + f.name + ",($)=>{let $$=$;return`"
}

func (f *funcDefine) orElse() string {
	return ""
}

func (f *funcDefine) end() string {
	return "`});"
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
