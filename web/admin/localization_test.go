package admin

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
)

func TestAdminLocalizedPages(t *testing.T) {
	for _, locale := range []struct{ code, signIn, restore, panel, users, roles, create, edit, editTitle, permissions, close string }{
		{"en", "Sign in", "Restore password", "Admin panel", "Users", "Roles", "Create role", "Edit role", `Edit role "Admin"`, "Permissions", "Close"},
		{"lv", "Pieslēgties", "Atjaunot paroli", "Administrācijas panelis", "Lietotāji", "Lomas", "Izveidot lomu", "Rediģēt lomu", "Rediģēt lomu «Admin»", "Atļaujas", "Aizvērt"},
		{"ru", "Войти", "Восстановить пароль", "Панель администратора", "Пользователи", "Роли", "Создать роль", "Редактировать роль", "Редактировать роль «Admin»", "Разрешения", "Закрыть"},
	} {
		t.Run(locale.code, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			cookies := login(t, mux)
			rolePath := "/roles/" + uuid.UUID(repository.role.ID()).String()

			for _, page := range []struct{ path, title string }{
				{"/sign-in", locale.signIn}, {"/restore-password", locale.restore},
				{"", locale.panel}, {"/users", locale.users}, {"/roles", locale.roles},
				{"/roles/create", locale.create}, {rolePath + "/edit", locale.edit}, {rolePath + "/view", "Admin"},
			} {
				for _, fragment := range []bool{false, true} {
					name := page.path

					if fragment {
						name += " fragment"
					}

					t.Run(name, func(t *testing.T) {
						request := httptest.NewRequest(http.MethodGet, "/admin/"+locale.code+page.path, nil)

						if page.path != "/sign-in" {
							for _, cookie := range cookies {
								request.AddCookie(cookie)
							}
						}

						if fragment {
							request.Header.Set("HX-Request", "true")
						}

						response := httptest.NewRecorder()
						mux.ServeHTTP(response, request)
						body := response.Body.String()

						if response.Code != http.StatusOK || response.Header().Get("Content-Language") != locale.code || !strings.Contains(body, page.title) {
							t.Fatalf("localized page = %d %s", response.Code, body)
						}

						if page.path == rolePath+"/view" && (!strings.Contains(body, `aria-label="`+locale.edit+`"`) || !strings.Contains(body, "</i> "+locale.edit+"</a>")) {
							t.Fatal("edit button must show only the localized action without the role name")
						}

						if page.path == rolePath+"/edit" && !strings.Contains(body, `<h1 id="page-heading" class="page-title__heading">`+html.EscapeString(locale.editTitle)+`</h1>`) {
							t.Fatal("edit page heading must include the quoted role name")
						}

						if !fragment && !strings.Contains(body, `<html lang="`+locale.code+`">`) {
							t.Fatal("document language differs from route")
						}

						if strings.Count(body, `class="language-switcher__link"`) != 3 || strings.Count(body, `aria-current="true"`) != 1 {
							t.Fatal("language switcher is missing or has multiple active languages")
						}

						for _, code := range []string{"en", "lv", "ru"} {
							if !strings.Contains(body, `href="/admin/`+code+page.path+`" lang="`+code+`"`) {
								t.Fatalf("language switcher lost the current route for %s", code)
							}
						}

						if strings.HasPrefix(page.path, "/roles") {
							if !strings.Contains(body, locale.permissions) || (page.path != "/roles/create" && !strings.Contains(body, `aria-label="`+locale.close+`"`)) {
								t.Fatal("role permissions or dialog labels were not translated")
							}
						}

						if locale.code != "en" && (strings.Contains(body, ">Sign out<") || strings.Contains(body, ">Cancel<") || strings.Contains(body, ">No matching options.<")) {
							t.Fatal("English interface copy leaked into translated page")
						}
					})
				}
			}
		})
	}
}

func TestAdminLocalizedMessages(t *testing.T) {
	for _, locale := range []struct{ code, invalidSession, roleValues, roleMissing, roleUsed, created string }{
		{"en", "The email, password, or session is invalid.", "Enter a name in at least one language.", "Role not found.", "Reassign those users", `Role "Editors" created successfully.`},
		{"lv", "E-pasts, parole vai sesija nav derīga.", "Ievadiet nosaukumu vismaz vienā valodā.", "Loma nav atrasta.", "piešķiriet šiem lietotājiem citu lomu", `Loma «Editors» veiksmīgi izveidota.`},
		{"ru", "Электронная почта, пароль или сеанс недействительны.", "Введите название хотя бы на одном языке.", "Роль не найдена.", "Назначьте им другую роль", `Роль «Editors» успешно создана.`},
	} {
		t.Run(locale.code, func(t *testing.T) {
			mux, repository, _ := httpFixture(t)
			root := "/admin/" + locale.code
			bad := httpRequest(mux, http.MethodPost, root+"/sign-in", "email=person%40example.com&password=wrong", nil)

			if bad.Code != http.StatusUnauthorized || !strings.Contains(html.UnescapeString(bad.Body.String()), locale.invalidSession) {
				t.Fatalf("sign-in error = %d %s", bad.Code, bad.Body.String())
			}

			cookies := login(t, mux)
			invalid := httpRequest(mux, http.MethodPost, root+"/roles/create", "name[en]=", cookies)
			missing := httpRequest(mux, http.MethodGet, root+"/roles/"+newTestUUID(t).String()+"/view", "", cookies)

			for _, check := range []struct {
				response *httptest.ResponseRecorder
				status   int
				message  string
			}{
				{invalid, http.StatusUnprocessableEntity, locale.roleValues}, {missing, http.StatusNotFound, locale.roleMissing},
			} {
				if check.response.Code != check.status || !strings.Contains(html.UnescapeString(check.response.Body.String()), check.message) {
					t.Fatalf("role error = %d %s", check.response.Code, check.response.Body.String())
				}
			}

			repository.deleteErr = users.ErrRoleAlreadyInUse
			request := httptest.NewRequest(http.MethodPost, root+"/roles/"+uuid.UUID(repository.role.ID()).String()+"/delete", strings.NewReader("confirm=delete"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("HX-Request", "true")

			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}

			failedDelete := httptest.NewRecorder()
			mux.ServeHTTP(failedDelete, request)

			if failedDelete.Code != http.StatusConflict || !strings.Contains(failedDelete.Body.String(), locale.roleUsed) || strings.Contains(failedDelete.Body.String(), "<html") {
				t.Fatalf("delete fragment = %d %s", failedDelete.Code, failedDelete.Body.String())
			}

			created := httpRequest(mux, http.MethodPost, root+"/roles/create", url.Values{"name[en]": {"Editors"}}.Encode(), cookies)
			success := httpRequest(mux, http.MethodGet, created.Header().Get("Location"), "", cookies)

			if created.Code != http.StatusSeeOther || success.Code != http.StatusOK || !strings.Contains(html.UnescapeString(success.Body.String()), locale.created) {
				t.Fatalf("success message = %d %s", success.Code, success.Body.String())
			}
		})
	}
}

func TestAdminLanguageSwitchFiltersAndFallback(t *testing.T) {
	mux, _, _ := httpFixture(t)
	cookies := login(t, mux)
	query := "permissions=view_user&permissions=manage_user&name=%C4%80&page=2&sort=name&order=desc"
	response := httpRequest(mux, http.MethodGet, "/admin/lv/roles?"+query, "", cookies)

	for _, code := range []string{"en", "lv", "ru"} {
		if !strings.Contains(response.Body.String(), `href="`+html.EscapeString("/admin/"+code+"/roles?"+query)+`" lang="`+code+`"`) {
			t.Fatalf("language link lost filters for %s", code)
		}
	}

	fallback := httpRequest(mux, http.MethodGet, "/admin/unknown/sign-in", "", nil)

	if fallback.Code != http.StatusOK || fallback.Header().Get("Content-Language") != "en" || !strings.Contains(fallback.Body.String(), `<html lang="en">`) || !strings.Contains(fallback.Body.String(), ">Sign in<") {
		t.Fatalf("unknown locale did not fall back to English: %d %s", fallback.Code, fallback.Body.String())
	}
}

func TestAdminLocalizedCrossOriginErrors(t *testing.T) {
	for _, tt := range []struct{ code, message string }{
		{"en", "This request was blocked."},
		{"lv", "Šis pieprasījums tika bloķēts."},
		{"ru", "Этот запрос заблокирован."},
	} {
		t.Run(tt.code, func(t *testing.T) {
			mux, _, _ := httpFixture(t)
			request := httptest.NewRequest(http.MethodPost, "https://admin.example.com/admin/"+tt.code+"/sign-in", nil)
			request.Header.Set("Origin", "https://attacker.example")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden || response.Header().Get("Content-Language") != tt.code || !strings.Contains(response.Body.String(), tt.message) || len(response.Result().Cookies()) != 0 {
				t.Fatalf("cross-origin error = %d %s", response.Code, response.Body.String())
			}
		})
	}
}
