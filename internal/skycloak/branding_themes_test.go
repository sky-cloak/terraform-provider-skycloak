package skycloak

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const themeUID = "44444444-4444-4444-4444-444444444444"

func ptrBool(b bool) *bool { return &b }

func TestLoginBrandingUpsertGetDelete(t *testing.T) {
	body := `{"cluster_id":"` + cuid + `","realm":"app","primary_color":"#0ea5e9","logo_url":"https://cdn/logo.png",` +
		`"forgot_password_enabled":true,"registration_enabled":false,"remember_me_enabled":true,"show_powered_by":false,` +
		`"internationalization":{"enabled":true,"default_locale":"en","supported_locales":["en","fr"],` +
		`"language_selection_mode":"automatic_with_selector","language_selector_position":"top_right","language_selector_style":"dropdown"},` +
		`"sso":{"enabled":true,"button_size":"medium","display_style":"logo_with_text","layout":"horizontal"},` +
		`"status":"applied","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodGet:
			writeJSON(w, 200, body)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.UpsertLoginBranding(context.Background(), cuid, "app", UpsertLoginBrandingRequest{
		PrimaryColor: "#0ea5e9", LogoURL: "https://cdn/logo.png",
		ForgotPasswordEnabled: ptrBool(true), RegistrationEnabled: ptrBool(false),
		Internationalization: &LoginI18n{Enabled: true, DefaultLocale: "en", SupportedLocales: []string{"en", "fr"}, LanguageSelectionMode: "automatic_with_selector", LanguageSelectorPosition: "top_right", LanguageSelectorStyle: "dropdown"},
		SSO:                  &SSOConfig{Enabled: true, ButtonSize: "medium", DisplayStyle: "logo_with_text", Layout: "horizontal"},
	})
	if err != nil || got.PrimaryColor != "#0ea5e9" || !got.ForgotPasswordEnabled || got.RegistrationEnabled {
		t.Fatalf("UpsertLoginBranding: %+v, %v", got, err)
	}
	if got.Internationalization == nil || got.Internationalization.LanguageSelectorStyle != "dropdown" {
		t.Fatalf("login i18n not mapped: %+v", got.Internationalization)
	}
	if got.SSO == nil || got.SSO.DisplayStyle != "logo_with_text" {
		t.Fatalf("login sso not mapped: %+v", got.SSO)
	}
	if _, err := c.GetLoginBranding(context.Background(), cuid, "app"); err != nil {
		t.Fatalf("GetLoginBranding: %v", err)
	}
	if err := c.DeleteLoginBranding(context.Background(), cuid, "app"); err != nil {
		t.Fatalf("DeleteLoginBranding: %v", err)
	}
}

func TestEmailBrandingUpsertGetDelete(t *testing.T) {
	body := `{"cluster_id":"` + cuid + `","realm":"app","primary_color":"#111827","footer_company_name":"Acme",` +
		`"internationalization":{"enabled":true,"default_locale":"en","supported_locales":["en"]},` +
		`"status":"applied","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut, http.MethodGet:
			writeJSON(w, 200, body)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.UpsertEmailBranding(context.Background(), cuid, "app", UpsertEmailBrandingRequest{
		PrimaryColor: "#111827", FooterCompanyName: "Acme",
		Internationalization: &EmailI18n{Enabled: true, DefaultLocale: "en", SupportedLocales: []string{"en"}},
	})
	if err != nil || got.PrimaryColor != "#111827" || got.FooterCompanyName != "Acme" {
		t.Fatalf("UpsertEmailBranding: %+v, %v", got, err)
	}
	if got.Internationalization == nil || !got.Internationalization.Enabled {
		t.Fatalf("email i18n not mapped: %+v", got.Internationalization)
	}
	if _, err := c.GetEmailBranding(context.Background(), cuid, "app"); err != nil {
		t.Fatalf("GetEmailBranding: %v", err)
	}
	if err := c.DeleteEmailBranding(context.Background(), cuid, "app"); err != nil {
		t.Fatalf("DeleteEmailBranding: %v", err)
	}
}

func TestThemesListGet(t *testing.T) {
	theme := `{"id":"` + themeUID + `","cluster_id":"` + cuid + `","name":"corporate","description":"Brand theme","version":"1.0",` +
		`"status":"deployed","theme_types":["login","email"],"file_size":2048,"deployed_at":"2026-01-01T00:00:00Z",` +
		`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/" + cuid + "/themes":
			writeJSON(w, 200, "["+theme+"]")
		case "/clusters/" + cuid + "/themes/" + themeUID:
			writeJSON(w, 200, theme)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	list, err := c.ListThemes(context.Background(), cuid)
	if err != nil || len(list) != 1 || list[0].Name != "corporate" || len(list[0].ThemeTypes) != 2 {
		t.Fatalf("ListThemes: %+v, %v", list, err)
	}
	got, err := c.GetTheme(context.Background(), cuid, themeUID)
	if err != nil || got.Status != "deployed" || got.FileSize != 2048 {
		t.Fatalf("GetTheme: %+v, %v", got, err)
	}
}

func TestThemeAssignmentSetGet(t *testing.T) {
	// login set to a theme, the rest null (default).
	body := `{"login":"` + themeUID + `","account":null,"admin":null,"email":null}`
	var sentNullEmail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), `"email":null`) {
			sentNullEmail = true
		}
		writeJSON(w, 200, body)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.SetThemeAssignment(context.Background(), cuid, "app", ThemeAssignment{Login: themeUID})
	if err != nil || got.Login != themeUID || got.Email != "" {
		t.Fatalf("SetThemeAssignment: %+v, %v", got, err)
	}
	if !sentNullEmail {
		t.Fatalf("expected empty fields to be sent as explicit null")
	}
	got, err = c.GetThemeAssignment(context.Background(), cuid, "app")
	if err != nil || got.Login != themeUID || got.Admin != "" {
		t.Fatalf("GetThemeAssignment: %+v, %v", got, err)
	}
}

func TestClientThemeAssignmentSetGet(t *testing.T) {
	body := `{"login":"` + themeUID + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, body)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.SetClientThemeAssignment(context.Background(), cuid, "app", "web", themeUID)
	if err != nil || got.Login != themeUID {
		t.Fatalf("SetClientThemeAssignment: %+v, %v", got, err)
	}
	if _, err := c.GetClientThemeAssignment(context.Background(), cuid, "app", "web"); err != nil {
		t.Fatalf("GetClientThemeAssignment: %v", err)
	}
}
