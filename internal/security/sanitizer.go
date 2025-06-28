package security

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

type SanitizerConfig struct {
	Enabled          bool `yaml:"enabled" mapstructure:"enabled"`
	MaxStringLength  int  `yaml:"max_string_length" mapstructure:"max_string_length"`
	MaxArrayLength   int  `yaml:"max_array_length" mapstructure:"max_array_length"`
	MaxObjectDepth   int  `yaml:"max_object_depth" mapstructure:"max_object_depth"`
	StrictMode       bool `yaml:"strict_mode" mapstructure:"strict_mode"`
	AllowHTML        bool `yaml:"allow_html" mapstructure:"allow_html"`
	AllowJavaScript  bool `yaml:"allow_javascript" mapstructure:"allow_javascript"`
	AllowSQLKeywords bool `yaml:"allow_sql_keywords" mapstructure:"allow_sql_keywords"`
}

type InputSanitizer struct {
	config     SanitizerConfig
	htmlPolicy *bluemonday.Policy
	sqlPattern *regexp.Regexp
	jsPattern  *regexp.Regexp
	xssPattern *regexp.Regexp
}

func NewInputSanitizer(config SanitizerConfig) *InputSanitizer {
	sanitizer := &InputSanitizer{
		config: config,
	}

	if config.Enabled {

		if config.AllowHTML {
			sanitizer.htmlPolicy = bluemonday.UGCPolicy()
		} else {
			sanitizer.htmlPolicy = bluemonday.StrictPolicy()
		}

		sanitizer.sqlPattern = regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute|script|declare|cast|convert|having|where|from|join|on|group\s+by|order\s+by|--|\||;|\*|'|"|\$|%|@|\+|=|<|>|\(|\)|,|\.|\\|/)`)

		sanitizer.jsPattern = regexp.MustCompile(`(?i)(javascript:|vbscript:|data:|onload|onerror|onclick|onmouseover|onfocus|onblur|onchange|onsubmit|<script|</script|eval\(|function\(|alert\(|confirm\(|prompt\()`)

		sanitizer.xssPattern = regexp.MustCompile(`(?i)(<script|</script|<iframe|</iframe|<object|</object|<embed|</embed|<link|<meta|<style|</style|javascript:|vbscript:|data:|on\w+\s*=)`)
	}

	return sanitizer
}

func (is *InputSanitizer) SanitizeString(input string) (string, error) {
	if !is.config.Enabled {
		return input, nil
	}

	if len(input) > is.config.MaxStringLength {
		if is.config.StrictMode {
			return "", fmt.Errorf("string length exceeds maximum allowed length of %d", is.config.MaxStringLength)
		}
		input = input[:is.config.MaxStringLength]
	}

	if !utf8.ValidString(input) {
		if is.config.StrictMode {
			return "", fmt.Errorf("invalid UTF-8 string")
		}
		input = strings.ToValidUTF8(input, "")
	}

	input = strings.ReplaceAll(input, "\x00", "")

	if !is.config.AllowSQLKeywords && is.sqlPattern.MatchString(input) {
		if is.config.StrictMode {
			return "", fmt.Errorf("potential SQL injection detected")
		}
		input = is.sqlPattern.ReplaceAllString(input, "")
	}

	if !is.config.AllowJavaScript && is.jsPattern.MatchString(input) {
		if is.config.StrictMode {
			return "", fmt.Errorf("potential JavaScript injection detected")
		}
		input = is.jsPattern.ReplaceAllString(input, "")
	}

	if is.xssPattern.MatchString(input) {
		if is.config.StrictMode {
			return "", fmt.Errorf("potential XSS detected")
		}
		input = is.xssPattern.ReplaceAllString(input, "")
	}

	if is.htmlPolicy != nil {
		input = is.htmlPolicy.Sanitize(input)
	}

	if !is.config.AllowHTML {
		input = html.EscapeString(input)
	}

	return strings.TrimSpace(input), nil
}

func (is *InputSanitizer) SanitizeValue(value any) (any, error) {
	return is.sanitizeValueWithDepth(value, 0)
}

func (is *InputSanitizer) sanitizeValueWithDepth(value any, depth int) (any, error) {
	if !is.config.Enabled {
		return value, nil
	}

	if depth > is.config.MaxObjectDepth {
		if is.config.StrictMode {
			return nil, fmt.Errorf("object depth exceeds maximum allowed depth of %d", is.config.MaxObjectDepth)
		}
		return nil, nil
	}

	switch v := value.(type) {
	case string:
		return is.SanitizeString(v)

	case []any:
		return is.sanitizeArray(v, depth)

	case map[string]any:
		return is.sanitizeObject(v, depth)

	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return v, nil

	case float32, float64:
		return v, nil

	case bool:
		return v, nil

	case nil:
		return nil, nil

	default:

		str := fmt.Sprintf("%v", v)
		return is.SanitizeString(str)
	}
}

func (is *InputSanitizer) sanitizeArray(arr []any, depth int) ([]any, error) {
	if len(arr) > is.config.MaxArrayLength {
		if is.config.StrictMode {
			return nil, fmt.Errorf("array length exceeds maximum allowed length of %d", is.config.MaxArrayLength)
		}
		arr = arr[:is.config.MaxArrayLength]
	}

	sanitized := make([]any, 0, len(arr))
	for _, item := range arr {
		sanitizedItem, err := is.sanitizeValueWithDepth(item, depth+1)
		if err != nil {
			if is.config.StrictMode {
				return nil, err
			}
			continue
		}
		sanitized = append(sanitized, sanitizedItem)
	}

	return sanitized, nil
}

func (is *InputSanitizer) sanitizeObject(obj map[string]any, depth int) (map[string]any, error) {
	sanitized := make(map[string]any)

	for key, value := range obj {

		sanitizedKey, err := is.SanitizeString(key)
		if err != nil {
			if is.config.StrictMode {
				return nil, fmt.Errorf("invalid key '%s': %w", key, err)
			}
			continue
		}

		if sanitizedKey == "" {
			continue
		}

		sanitizedValue, err := is.sanitizeValueWithDepth(value, depth+1)
		if err != nil {
			if is.config.StrictMode {
				return nil, fmt.Errorf("invalid value for key '%s': %w", key, err)
			}
			continue
		}

		sanitized[sanitizedKey] = sanitizedValue
	}

	return sanitized, nil
}

func (is *InputSanitizer) SanitizeURL(rawURL string) (string, error) {
	if !is.config.Enabled {
		return rawURL, nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	allowedSchemes := []string{"http", "https", "ftp", "ftps"}
	if !contains(allowedSchemes, strings.ToLower(parsedURL.Scheme)) {
		if is.config.StrictMode {
			return "", fmt.Errorf("disallowed URL scheme: %s", parsedURL.Scheme)
		}
		return "", nil
	}

	if parsedURL.Host != "" {
		host, err := is.SanitizeString(parsedURL.Host)
		if err != nil {
			return "", fmt.Errorf("invalid host: %w", err)
		}
		parsedURL.Host = host
	}

	if parsedURL.Path != "" {
		path, err := is.SanitizeString(parsedURL.Path)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
		parsedURL.Path = path
	}

	if parsedURL.RawQuery != "" {
		query := parsedURL.Query()
		sanitizedQuery := url.Values{}

		for key, values := range query {
			sanitizedKey, err := is.SanitizeString(key)
			if err != nil {
				if is.config.StrictMode {
					return "", fmt.Errorf("invalid query key '%s': %w", key, err)
				}
				continue
			}

			for _, value := range values {
				sanitizedValue, err := is.SanitizeString(value)
				if err != nil {
					if is.config.StrictMode {
						return "", fmt.Errorf("invalid query value '%s': %w", value, err)
					}
					continue
				}
				sanitizedQuery.Add(sanitizedKey, sanitizedValue)
			}
		}

		parsedURL.RawQuery = sanitizedQuery.Encode()
	}

	return parsedURL.String(), nil
}

func (is *InputSanitizer) SanitizeFilename(filename string) (string, error) {
	if !is.config.Enabled {
		return filename, nil
	}

	filename = strings.ReplaceAll(filename, "..", "")
	filename = strings.ReplaceAll(filename, "/", "")
	filename = strings.ReplaceAll(filename, "\\", "")

	filename = strings.ReplaceAll(filename, "\x00", "")

	filename = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, filename)

	sanitized, err := is.SanitizeString(filename)
	if err != nil {
		return "", err
	}

	reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	upperName := strings.ToUpper(sanitized)
	for _, reserved := range reservedNames {
		if upperName == reserved {
			if is.config.StrictMode {
				return "", fmt.Errorf("reserved filename: %s", sanitized)
			}
			sanitized = "_" + sanitized
			break
		}
	}

	return sanitized, nil
}

func (is *InputSanitizer) ValidateEmail(email string) (string, error) {
	if !is.config.Enabled {
		return email, nil
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	sanitized, err := is.SanitizeString(email)
	if err != nil {
		return "", err
	}

	if !emailRegex.MatchString(sanitized) {
		return "", fmt.Errorf("invalid email format")
	}

	return strings.ToLower(sanitized), nil
}

func (is *InputSanitizer) ValidateNumber(input string, minVal, maxVal float64) (float64, error) {
	if !is.config.Enabled {
		return strconv.ParseFloat(input, 64)
	}

	sanitized, err := is.SanitizeString(input)
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseFloat(sanitized, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number format: %w", err)
	}

	if value < minVal || value > maxVal {
		return 0, fmt.Errorf("number %f is outside allowed range [%f, %f]", value, minVal, maxVal)
	}

	return value, nil
}

func (is *InputSanitizer) IsEnabled() bool {
	return is.config.Enabled
}

func (is *InputSanitizer) SanitizeMiddleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !is.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			query := r.URL.Query()
			sanitizedQuery := url.Values{}

			for key, values := range query {
				sanitizedKey, err := is.SanitizeString(key)
				if err != nil {
					http.Error(w, fmt.Sprintf("Invalid query parameter key: %s", key), http.StatusBadRequest)
					return
				}

				for _, value := range values {
					sanitizedValue, err := is.SanitizeString(value)
					if err != nil {
						http.Error(w, fmt.Sprintf("Invalid query parameter value: %s", value), http.StatusBadRequest)
						return
					}
					sanitizedQuery.Add(sanitizedKey, sanitizedValue)
				}
			}

			r.URL.RawQuery = sanitizedQuery.Encode()

			for key, values := range r.Header {

				if isStandardHeader(key) {
					continue
				}

				for i, value := range values {
					sanitized, err := is.SanitizeString(value)
					if err != nil {
						http.Error(w, fmt.Sprintf("Invalid header value: %s", key), http.StatusBadRequest)
						return
					}
					r.Header[key][i] = sanitized
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func isStandardHeader(header string) bool {
	standardHeaders := []string{
		"Authorization", "Content-Type", "Content-Length", "Accept", "Accept-Encoding",
		"Accept-Language", "Cache-Control", "Connection", "Cookie", "Host", "Referer",
		"User-Agent", "X-Requested-With", "X-Forwarded-For", "X-Real-IP",
	}

	return contains(standardHeaders, header)
}

type EntitySanitizer struct {
	sanitizer *InputSanitizer
}

func NewEntitySanitizer(config SanitizerConfig) *EntitySanitizer {
	return &EntitySanitizer{
		sanitizer: NewInputSanitizer(config),
	}
}

func (es *EntitySanitizer) SanitizeEntityProperties(properties map[string]any) (map[string]any, error) {
	sanitized, err := es.sanitizer.SanitizeValue(properties)
	if err != nil {
		return nil, err
	}

	if sanitizedMap, ok := sanitized.(map[string]any); ok {
		return sanitizedMap, nil
	}

	return nil, fmt.Errorf("sanitized value is not a map")
}

func (es *EntitySanitizer) SanitizeEntityType(entityType string) (string, error) {

	sanitized, err := es.sanitizer.SanitizeString(entityType)
	if err != nil {
		return "", err
	}

	if len(sanitized) == 0 || len(sanitized) > 100 {
		return "", fmt.Errorf("entity type length must be between 1 and 100 characters")
	}

	validPattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
	if !validPattern.MatchString(sanitized) {
		return "", fmt.Errorf("entity type contains invalid characters")
	}

	return sanitized, nil
}

func (es *EntitySanitizer) SanitizeURN(urn string) (string, error) {
	sanitized, err := es.sanitizer.SanitizeString(urn)
	if err != nil {
		return "", err
	}

	if len(sanitized) == 0 || len(sanitized) > 500 {
		return "", fmt.Errorf("URN length must be between 1 and 500 characters")
	}

	return sanitized, nil
}

type SearchSanitizer struct {
	sanitizer *InputSanitizer
}

func NewSearchSanitizer(config SanitizerConfig) *SearchSanitizer {
	return &SearchSanitizer{
		sanitizer: NewInputSanitizer(config),
	}
}

func (ss *SearchSanitizer) SanitizeSearchQuery(query string) (string, error) {
	sanitized, err := ss.sanitizer.SanitizeString(query)
	if err != nil {
		return "", err
	}

	if len(sanitized) > 1000 {
		return "", fmt.Errorf("search query too long")
	}

	return sanitized, nil
}

func (ss *SearchSanitizer) SanitizeSearchFilters(filters map[string]any) (map[string]any, error) {
	sanitized, err := ss.sanitizer.SanitizeValue(filters)
	if err != nil {
		return nil, err
	}

	if sanitizedMap, ok := sanitized.(map[string]any); ok {
		return sanitizedMap, nil
	}

	return nil, fmt.Errorf("sanitized value is not a map")
}
