package main

import (
	"encoding/json"
	"fmt"
	"github.com/samuel/go-gettext/gettext"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

func IsParamSet(r *http.Request, param string) bool {
	return len(r.URL.Query().Get(param)) > 0
}

func Lang(r *http.Request) string {
	lang := r.URL.Query().Get("lang")
	if len(lang) == 0 {
		lang = "en_US"
	}
	return lang
}

func GetQS(q url.Values, param string, deflt int) (num int, str string) {
	str = q.Get(param)
	num, err := strconv.Atoi(str)
	if err != nil {
		num = deflt
		str = ""
	} else {
		str = fmt.Sprintf("&%s=%s", param, str)
	}
	return
}

func GetHost(r *http.Request) (host string, err error) {
	// get remote ip
	host = r.Header.Get("X-Forwarded-For")
	if len(host) > 0 {
		parts := strings.Split(host, ",")
		// apache will append the remote address
		host = strings.TrimSpace(parts[len(parts)-1])
	} else {
		host, _, err = net.SplitHostPort(r.RemoteAddr)
	}
	return
}

var TBBUserAgents = regexp.MustCompile(`^Mozilla/5\.0 \([^)]*\) Gecko/([\d]+\.0|20100101) Firefox/[\d]+\.0$`)

func LikelyTBB(ua string) bool {
	return TBBUserAgents.MatchString(ua)
}

func FuncMap(domain *gettext.Domain) template.FuncMap {
	return template.FuncMap{
		"UnEscaped": func(x string) interface{} {
			return template.HTML(x)
		},
		"UnEscapedURL": func(x string) interface{} {
			return template.URL(x)
		},
		"GetText": func(lang string, text string) string {
			return domain.GetText(lang, text)
		},
		"Equal": func(one string, two string) bool {
			return one == two
		},
		"Not": func(b bool) bool {
			return !b
		},
		"And": func(a bool, b bool) bool {
			return a && b
		},
	}
}

var Layout *template.Template

func CompileTemplate(base string, domain *gettext.Domain, templateName string) *template.Template {
	if Layout == nil {
		Layout = template.New("")
		Layout = Layout.Funcs(FuncMap(domain))
		Layout = template.Must(Layout.ParseFiles(
			path.Join(base, "public/base.html"),
			path.Join(base, "public/torbutton.html"),
		))
	}
	l, err := Layout.Clone()
	if err != nil {
		log.Fatal(err)
	}
	return template.Must(l.ParseFiles(path.Join(base, "public/", templateName)))
}

type locale struct {
	Code string
	Name string
}

func GetLocaleList(base string) map[string]string {
	// populated from https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes
	// and https://en.wikipedia.org/w/api.php?action=sitematrix&format=json
	haveTranslatedNames := map[string]string{
		"ar":    "??????????????",
		"bg":    "??????????????????",
		"bn":    "???????????????",
		"bs":    "Bosanski jezik",
		"ca":    "Catal??",
		"cs":    "??e??tina",
		"da":    "Dansk",
		"de":    "Deutsch",
		"el":    "????????????????",
		"en_GB": "English (United Kingdom)",
		"eo":    "Esperanto",
		"es":    "Espa??ol",
		"es_AR": "Espa??ol (Argentina)",
		"es_MX": "Espa??ol (Mexico)",
		"et":    "Eesti",
		"eu":    "Euskara",
		"fa":    "??????????",
		"fi":    "Suomi",
		"fr":    "Fran??ais",
		"ga":    "Gaeilge",
		"he":    "??????????",
		"hi":    "??????????????????",
		"hr":    "Hrvatski jezik",
		"hr_HR": "Hrvatski jezik (Croatia)",
		"hu":    "Magyar",
		"id":    "Bahasa Indonesia",
		"is":    "??slenska",
		"it":    "Italiano",
		"ja":    "?????????",
		"ka":    "?????????????????????",
		"ko":    "?????????",
		"lt":    "lietuvi?? kalba",
		"lv":    "Latvie??u valoda",
		"mk":    "???????????????????? ??????????",
		"ms_MY": "Bahasa Melayu",
		"nb":    "Norsk bokm??l",
		"nl":    "Nederlands",
		"nl_BE": "Vlaams",
		"nn":    "Norsk nynorsk",
		"pa":    "??????????????????",
		"pl":    "J??zyk polski",
		"pt":    "Portugu??s",
		"pt_BR": "Portugu??s brasileiro",
		"pt_PT": "Portugu??s europeu",
		"ro":    "rom??n??",
		"ru":    "?????????????? ????????",
		"sk":    "Sloven??ina",
		"sq":    "shqip",
		"sr":    "???????????? ??????????",
		"sv":    "Svenska",
		"ta":    "???????????????",
		"th":    "?????????",
		"tr":    "T??rk??e",
		"uk":    "???????????????????? ????????",
		"vi":    "Ti???ng Vi???t",
		"zh_CN": "????????????",
		"zh_HK": "????????????",
		"zh_TW": "????????????",
	}

	// for all folders in locale which match a locale from https://www.transifex.com/api/2/languages/
	// use the language name unless we have an override
	webLocales, err := FetchTranslationLocales(base)
	if err != nil {
		log.Printf("Failed to get up to date language list, using fallback.")
		return haveTranslatedNames
	}

	return GetInstalledLocales(base, webLocales, haveTranslatedNames)
}

func FetchTranslationLocales(base string) (map[string]locale, error) {
	file, err := os.Open(path.Join(base, "data/langs"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	webLocales := make(map[string]locale)
	// Parse the api response into a list of possible locales
	dec := json.NewDecoder(file)
	for {
		var webList []locale
		if err = dec.Decode(&webList); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		// The api returns an array, so we need to map it
		for _, l := range webList {
			webLocales[l.Code] = l
		}
	}

	return webLocales, nil
}

// Get a list of all languages installed in our locale folder with translations if available
func GetInstalledLocales(base string, webLocales map[string]locale, nameTranslations map[string]string) map[string]string {
	localFiles, err := ioutil.ReadDir(path.Join(base, "locale"))

	if err != nil {
		log.Print("No locales found in 'locale'. Try running 'make i18n'.")
		log.Fatal(err)
	}

	locales := make(map[string]string, len(localFiles))
	locales["en_US"] = "English"

	for _, f := range localFiles {
		// TODO: Ensure a language has 100% of the template file
		// Currently this is what should be on the torcheck_completed
		// branch on the translations git should be, so we don't really
		// have to check it in theory...
		code := f.Name()

		// Only accept folders which have corresponding locale
		if !f.IsDir() || webLocales[code] == (locale{}) {
			continue
		}

		// If we have a translated name for a given locale, use it
		if transName := nameTranslations[code]; transName != "" {
			locales[code] = transName
		} else {
			log.Print("No translated name for code: " + code)
			locales[code] = webLocales[code].Name
		}
	}

	return locales
}
