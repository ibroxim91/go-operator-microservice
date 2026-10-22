package services

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Operator tokeni URL ichida saqlanmaydi: so'rov yuborilishidan bir lahza
// oldin SignURL qo'shadi. Shu sababli logga, api_querycache jadvaliga va
// Redis'dagi share havolalariga token tushmaydi, token almashtirilganda esa
// eski saqlangan URL'lar ham ishlayveradi.

const oauthTokenParam = "oauth_token"

var oauthTokenPattern = regexp.MustCompile(`oauth_token=[^&\s"']*`)

func operatorConfig(operator string) (SamoServiceConfig, bool) {
	for _, cfg := range (&SamoService{}).getServiceConfigs() {
		if cfg.Name == operator {
			return cfg, true
		}
	}
	return SamoServiceConfig{}, false
}

// OperatorToken — operatorning tokeni (.env dan), topilmasa bo'sh satr.
func OperatorToken(operator string) string {
	cfg, _ := operatorConfig(operator)
	return cfg.OAuthToken
}

// SignURL — so'rov yuborishdan oldin URL'ga operator tokenini qo'shadi.
// URL'da eski token bo'lsa (bazadagi eski yozuvlar), u almashtiriladi.
//
// Token faqat operatorning o'z manziliga (BaseURL bilan bir xil sxema,
// host va yo'l) qo'shiladi. POST /search-tours URL'ni so'rov tanasidan
// oladi va internetga ochiq — tekshiruvsiz har qanday hostga token
// yuborib yuborilardi.
func SignURL(operator, rawURL string) (string, error) {
	cfg, ok := operatorConfig(operator)
	if !ok || cfg.OAuthToken == "" {
		return "", errors.New("no oauth token configured for operator " + operator)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil || base.Host == "" {
		return "", errors.New("invalid base url for operator " + operator)
	}
	if !sameEndpoint(rawURL, cfg.BaseURL) {
		return "", errors.New("url does not belong to operator " + operator)
	}
	token := cfg.OAuthToken
	q := u.Query()
	q.Set(oauthTokenParam, token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// sameEndpoint — URL operatorning hozirgi BaseURL'iga tegishlimi. Cache'dagi
// eski URL boshqa manzilga ishora qilsa, SignURL uni rad etadi va qidiruv
// natijasiz qoladi — shuning uchun bunday yozuv o'rniga URL qayta quriladi.
func sameEndpoint(rawURL, baseURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return u.Scheme == base.Scheme && strings.EqualFold(u.Host, base.Host) && u.Path == base.Path && u.User == nil
}

// StripToken — URL'dan tokenni olib tashlaydi (saqlash va log uchun).
func StripToken(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return RedactToken(rawURL)
	}
	q := u.Query()
	if !q.Has(oauthTokenParam) {
		return rawURL
	}
	q.Del(oauthTokenParam)
	u.RawQuery = q.Encode()
	return u.String()
}

// RedactToken — ixtiyoriy matndagi token qiymatini yashiradi.
func RedactToken(s string) string {
	return oauthTokenPattern.ReplaceAllString(s, oauthTokenParam+"=REDACTED")
}

// RedactError — http.Client xatosi (*url.Error) ichidagi to'liq URL'ni
// tokensiz qiladi; aks holda token xato matni orqali logga tushadi.
func RedactError(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		clone := *ue
		clone.URL = StripToken(ue.URL)
		return &clone
	}
	return err
}
