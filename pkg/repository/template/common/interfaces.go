package common

// TemplateEngine defines the common interface for all template engines
type TemplateEngine interface {
	RegisterTemplate(name, content string) error
	ExecuteTemplate(name string, data interface{}) (string, error)
}

// TemplateProcessor defines the common interface for template processors
type TemplateProcessor interface {
	Process(content string, config interface{}) string
}

// TemplatePool defines the common interface for template pools
type TemplatePool interface {
	Get() interface{}
	Put(item interface{})
}

// TemplateCache defines the common interface for template caches
type TemplateCache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	Clear()
}
