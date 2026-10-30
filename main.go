// aishimbun: turns daily AI digests (JSON) into the AI朝刊 static site.
//
//	go run . -site issues -out _site           # whole site (index, /YYYY/MM/DD/, archive, feed, sitemap, CNAME)
//	go run . -in issues/digest-2026-10-30.json # single page -> ai-shimbun-2026-10-30.html next to the input
//	go run . -in X.json -render                # + PNG + PDF via render.mjs (node + playwright)
//	go run . -in X.json -og og/2026-10-30.jpg  # + 1400x735 OGP image via render.mjs
package main

import (
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed template.html
var tpl string

//go:embed style.css
var css string

type Item struct {
	Tomo     string   `json:"tomo"` // optional: the editor's own comment (知洋のひとこと)
	Tag      string   `json:"tag"`
	Kicker   string   `json:"kicker"`
	Flag     string   `json:"flag"`
	Region   string   `json:"region"`
	Vertical bool     `json:"vertical"`
	Headline string   `json:"headline"`
	Deck     string   `json:"deck"`
	Subheads []string `json:"subheads"`
	Body     []string `json:"body"`
	Imi      string   `json:"imi"`
	Links    []string `json:"links"`
	Stat     *Stat    `json:"stat"`
	Related  []Item   `json:"related"`
}
type Stat struct {
	Num     string `json:"num"`
	Caption string `json:"caption"`
}
type Brief struct {
	T string `json:"t"`
	U string `json:"u"`
}
type Group struct {
	Name  string  `json:"name"`
	Items []Brief `json:"items"`
}
type Column struct {
	Title string `json:"title"`
	Label string `json:"label"`
	Note  string `json:"note"`
	Text  string `json:"text"`
}
type Num struct {
	Num   string `json:"num"`
	Label string `json:"label"`
}
type Link struct{ Href, Short, Title string }
type Digest struct {
	Date  string `json:"date"`
	Issue int    `json:"issue"` // optional; defaults to day-of-year
	Paper struct {
		Name    string `json:"name"`
		Tagline string `json:"tagline"`
		Edition string `json:"edition"`
	} `json:"paper"`
	Lead    Item    `json:"lead"`
	Left    []Item  `json:"left"`
	Right   []Item  `json:"right"`
	Labs    []Item  `json:"labs"`
	Column  Column  `json:"column"`
	Society []Item  `json:"society"`
	Briefs  []Group `json:"briefs"`
	Numbers []Num   `json:"numbers"` // optional "きょうの数字" box

	// derived
	DateJP, WeekdayJP, Short string
	Reiwa                    int
	Total                    int
	CSS                      template.CSS

	// site-level (set by build)
	Title, Desc, Root, Canonical, OGImage string
	Prev, Next                            *Link
}

var wd = []string{"日", "月", "火", "水", "木", "金", "土"}
var statusRe = regexp.MustCompile(`^/([^/]+)/status/`)
var tcyRe = regexp.MustCompile(`[A-Za-z0-9]{1,4}`)

func label(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	if strings.HasPrefix(p.Path, "/i/trending/") {
		return "Xトレンド"
	}
	if m := statusRe.FindStringSubmatch(p.Path); m != nil {
		return "@" + m[1]
	}
	return p.Host
}

const baseURL = "https://news.abalol.xyz"

var funcs = template.FuncMap{
	"label": label,
	"first": func(l []string) string {
		if len(l) > 0 {
			return l[0]
		}
		return ""
	},
	"rest": func(l []string) []string {
		if len(l) > 1 {
			return l[1:]
		}
		return nil
	},
	"br": func(s string) template.HTML {
		return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
	},
	"flat": func(s string) string { return strings.ReplaceAll(s, "\n", "") },
	"tcy": func(s string) template.HTML {
		return template.HTML(tcyRe.ReplaceAllString(template.HTMLEscapeString(s), `<span class="tcy">$0</span>`))
	},
}

func load(path string) Digest {
	b, err := os.ReadFile(path)
	check(err)
	var d Digest
	check(json.Unmarshal(b, &d))
	t, err := time.Parse("2006-01-02", d.Date)
	if err != nil {
		check(fmt.Errorf("%s: bad date %q: %v", path, d.Date, err))
	}
	if d.Lead.Headline == "" {
		check(fmt.Errorf("%s: lead.headline is empty", path))
	}
	d.Reiwa = t.Year() - 2018
	d.DateJP = fmt.Sprintf("%d年（令和%d年）%d月%d日", t.Year(), d.Reiwa, int(t.Month()), t.Day())
	d.WeekdayJP = wd[t.Weekday()] + "曜日"
	d.Short = fmt.Sprintf("%d/%02d/%02d", t.Year(), int(t.Month()), t.Day())
	if d.Issue == 0 {
		d.Issue = t.YearDay()
	}
	for _, g := range d.Briefs {
		d.Total += len(g.Items)
	}
	d.CSS = template.CSS(css)
	d.Total += len(d.Labs) + len(d.Society) + len(d.Left) + len(d.Right) + 1 + len(d.Lead.Related)
	return d
}

func (d Digest) Path() string {
	return strings.ReplaceAll(d.Date, "-", "/") + "/"
}

func (d Digest) HeadlinePlain() string { return strings.ReplaceAll(d.Lead.Headline, "\n", "") }

func (d Digest) desc() string {
	s := d.HeadlinePlain()
	if d.Lead.Deck != "" {
		s += "　" + d.Lead.Deck
	}
	for _, x := range d.Lead.Subheads {
		s += "。" + x
	}
	r := []rune(s)
	if len(r) > 110 {
		s = string(r[:110]) + "…"
	}
	return s
}

func render(tp *template.Template, name string, data any, path string) {
	check(os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	check(err)
	check(tp.ExecuteTemplate(f, name, data))
	f.Close()
}

func copyFile(src, dst string) bool {
	in, err := os.Open(src)
	if err != nil {
		return false
	}
	defer in.Close()
	check(os.MkdirAll(filepath.Dir(dst), 0o755))
	out, err := os.Create(dst)
	check(err)
	defer out.Close()
	_, err = io.Copy(out, in)
	check(err)
	return true
}

func xe(s string) string {
	var b strings.Builder
	check(xml.EscapeText(&b, []byte(s)))
	return b.String()
}

type archPage struct {
	Digest
	Issues []Digest
}

func buildSite(dir, out, og string) {
	files, err := filepath.Glob(filepath.Join(dir, "digest-*.json"))
	check(err)
	var ds []Digest
	seen := map[string]string{}
	for _, f := range files {
		d := load(f)
		if want := "digest-" + d.Date + ".json"; filepath.Base(f) != want {
			check(fmt.Errorf("%s: file name does not match date %s", f, d.Date))
		}
		if p, ok := seen[d.Date]; ok {
			check(fmt.Errorf("duplicate date %s (%s, %s)", d.Date, p, f))
		}
		seen[d.Date] = f
		ds = append(ds, d)
	}
	if len(ds) == 0 {
		check(fmt.Errorf("no digest-*.json in %s", dir))
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].Date > ds[j].Date }) // newest first
	check(os.RemoveAll(out))
	check(os.MkdirAll(out, 0o755))
	tp, err := template.New("p").Funcs(funcs).Parse(tpl)
	check(err)
	_, err = tp.New("archive").Parse(archiveTpl)
	check(err)

	def := "og/default.jpg"
	copyFile(filepath.Join(og, "default.jpg"), filepath.Join(out, def))
	for i := range ds {
		d := &ds[i]
		d.Root = "../../../"
		d.Canonical = baseURL + "/" + d.Path()
		d.Title = d.Paper.Name + "　" + d.DateJP + d.WeekdayJP + "｜" + d.HeadlinePlain()
		d.Desc = d.desc()
		d.OGImage = baseURL + "/" + def
		if copyFile(filepath.Join(og, d.Date+".jpg"), filepath.Join(out, d.Path(), "og.jpg")) {
			d.OGImage = d.Canonical + "og.jpg"
		}
		if i+1 < len(ds) { // older
			o := ds[i+1]
			d.Prev = &Link{Href: d.Root + o.Path(), Short: o.Short}
		}
		if i > 0 { // newer
			n := ds[i-1]
			d.Next = &Link{Href: d.Root + n.Path(), Short: n.Short}
		}
		render(tp, "p", d, filepath.Join(out, d.Path(), "index.html"))
		fmt.Println("wrote", d.Path())
	}
	// index = latest issue (same content, canonical points at the dated URL)
	l := ds[0]
	l.Root = "./"
	if l.Prev != nil {
		p := *l.Prev
		p.Href = strings.TrimPrefix(p.Href, "../../../")
		l.Prev = &p
	}
	l.Next = nil
	render(tp, "p", &l, filepath.Join(out, "index.html"))
	fmt.Println("wrote index.html (latest:", l.Date+")")

	// archive
	ap := archPage{Issues: ds}
	ap.Paper = ds[0].Paper
	ap.Root, ap.Canonical = "../", baseURL+"/archive/"
	ap.Title = "アーカイブ｜" + ds[0].Paper.Name
	ap.Desc = ds[0].Paper.Name + "のバックナンバー一覧（新しい順）"
	ap.OGImage = baseURL + "/" + def
	ap.CSS = template.CSS(css)
	render(tp, "archive", &ap, filepath.Join(out, "archive", "index.html"))
	fmt.Println("wrote archive/index.html")

	// RSS
	var rb strings.Builder
	rb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom"><channel>` + "\n")
	rb.WriteString("<title>" + xe(ds[0].Paper.Name) + "</title><link>" + baseURL + "/</link>")
	rb.WriteString("<description>" + xe(ds[0].Paper.Name+"：世界のAIニュースを、毎朝ひとつの紙面に") + "</description><language>ja</language>")
	rb.WriteString(`<atom:link href="` + baseURL + `/feed.xml" rel="self" type="application/rss+xml"/>` + "\n")
	for i, d := range ds {
		if i >= 30 {
			break
		}
		t, _ := time.Parse("2006-01-02 15:04 -0700", d.Date+" 08:00 +0800")
		rb.WriteString("<item><title>" + xe(d.Short+"　"+d.HeadlinePlain()) + "</title><link>" + baseURL + "/" + d.Path() + "</link><guid isPermaLink=\"true\">" + baseURL + "/" + d.Path() + "</guid><pubDate>" + t.Format(time.RFC1123Z) + "</pubDate><description>" + xe(d.desc()) + "</description></item>\n")
	}
	rb.WriteString("</channel></rss>\n")
	check(os.WriteFile(filepath.Join(out, "feed.xml"), []byte(rb.String()), 0o644))

	// sitemap
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	sb.WriteString("<url><loc>" + baseURL + "/</loc><lastmod>" + ds[0].Date + "</lastmod></url>\n")
	sb.WriteString("<url><loc>" + baseURL + "/archive/</loc><lastmod>" + ds[0].Date + "</lastmod></url>\n")
	for _, d := range ds {
		sb.WriteString("<url><loc>" + baseURL + "/" + d.Path() + "</loc><lastmod>" + d.Date + "</lastmod></url>\n")
	}
	sb.WriteString("</urlset>\n")
	check(os.WriteFile(filepath.Join(out, "sitemap.xml"), []byte(sb.String()), 0o644))
	check(os.WriteFile(filepath.Join(out, "robots.txt"), []byte("User-agent: *\nAllow: /\nSitemap: "+baseURL+"/sitemap.xml\n"), 0o644))
	check(os.WriteFile(filepath.Join(out, "CNAME"), []byte("news.abalol.xyz\n"), 0o644))
	check(os.WriteFile(filepath.Join(out, ".nojekyll"), nil, 0o644))
	fmt.Println("site built:", len(ds), "issue(s) ->", out)
}

func main() {
	in := flag.String("in", "", "single digest JSON file")
	site := flag.String("site", "", "directory of digest-YYYY-MM-DD.json files: build the whole site")
	out := flag.String("out", "", "output dir (default: _site for -site, dir of -in otherwise)")
	og := flag.String("og", "", "with -in: also write a 1400x735 OGP jpg to this path (needs node+playwright)")
	ogdir := flag.String("ogdir", "og", "with -site: dir holding YYYY-MM-DD.jpg / default.jpg OGP images")
	rnd := flag.Bool("render", false, "with -in: also render PNG+PDF with node render.mjs")
	flag.Parse()
	if *site != "" {
		o := *out
		if o == "" {
			o = "_site"
		}
		buildSite(*site, o, *ogdir)
		return
	}
	if *in == "" {
		fmt.Fprintln(os.Stderr, "usage: aishimbun -site issues [-out _site] | -in digest.json [-out dir] [-render] [-og file.jpg]")
		os.Exit(2)
	}
	d := load(*in)
	d.Root, d.Canonical, d.Title, d.Desc = "./", baseURL+"/"+d.Path(), d.Paper.Name+"　"+d.DateJP+d.WeekdayJP, d.desc()
	d.OGImage = baseURL + "/og/default.jpg"
	tp, err := template.New("p").Funcs(funcs).Parse(tpl)
	check(err)
	dir := *out
	if dir == "" {
		dir = filepath.Dir(*in)
	}
	name := filepath.Join(dir, "ai-shimbun-"+d.Date+".html")
	render(tp, "p", &d, name)
	fmt.Println("wrote", name)
	if *rnd {
		run("node", "render.mjs", name)
	}
	if *og != "" {
		run("node", "render.mjs", name, "--og", *og)
	}
}

func run(args ...string) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	check(cmd.Run())
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

//go:embed archive.html
var archiveTpl string
