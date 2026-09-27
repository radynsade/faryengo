package languages

type Translation struct {
	languageCode LanguageCode
	content      string
}

type Text struct {
}

func (t Translation) LanguageCode() LanguageCode {
	return t.languageCode
}

func (t Translation) Content() string {
	return t.content
}
