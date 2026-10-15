package handlers

import (
	"net/http"

	"github.com/radynsade/faryengo/internal/app/input"
	"github.com/radynsade/faryengo/internal/currencies"
	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
	"github.com/radynsade/faryengo/web/admin/templates/components"
	"github.com/radynsade/faryengo/web/admin/templates/layouts"
	"github.com/radynsade/faryengo/web/admin/templates/pages"
	"github.com/radynsade/faryengo/web/admin/utils"
)

// Currencies have no separate view permission, so every page of the section
// needs permission to manage them.

//
// Currency list
//

// Currencies page

func (h *Handler) currenciesPage(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionCurrencies, users.PermissionManageCurrency, func(request *http.Request, _ *security.Identity, panel layouts.PanelProps) {
		props := pages.CurrencyListProps{Panel: panel}
		status := http.StatusOK

		list, err := h.currencies.List(request.Context())

		for _, currency := range list {
			props.Rows = append(props.Rows, utils.CurrencyRow(currency))
		}

		var flashErr error

		if err != nil {
			var messageID string

			status, messageID = utils.CurrencyError(request, err)
			props.Errors, flashErr = h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))
		}

		if flashErr != nil {
			utils.FlashUnavailable(writer, request, flashErr)
		} else {
			h.render(writer, request, status, panel.Title, pages.Currencies(props))
		}
	})
}

//
// Currency form
//

// Currency create

func (h *Handler) currencyCreate(writer http.ResponseWriter, request *http.Request) {
	h.currencyForm(writer, request, false)
}

// Currency edit

func (h *Handler) currencyEdit(writer http.ResponseWriter, request *http.Request) {
	h.currencyForm(writer, request, true)
}

//
// Currency details
//

// Currency view

func (h *Handler) currencyView(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionCurrencies, users.PermissionManageCurrency, func(request *http.Request, current *security.Identity, panel layouts.PanelProps) {
		currency, err := h.findCurrency(request)

		if err != nil {
			h.renderCurrencyError(writer, request, current, err)
		} else {
			props := pages.CurrencyViewProps{Panel: panel, Currency: utils.CurrencyRow(currency)}

			h.render(writer, request, http.StatusOK, props.Currency.Code, pages.CurrencyView(props))
		}
	})
}

// Currency delete

// A failed deletion keeps the confirmation dialog open with the error; only
// the dialog is replaced.

func (h *Handler) currencyDelete(writer http.ResponseWriter, request *http.Request) {
	h.withPanel(writer, request, layouts.SectionCurrencies, users.PermissionManageCurrency, func(request *http.Request, _ *security.Identity, panel layouts.PanelProps) {
		code, err := utils.ParseCurrencyCode(request)

		if err == nil {
			err = utils.ParseDeleteForm(writer, request)
		}

		if err == nil {
			err = h.currencies.Delete(request.Context(), string(code))
		}

		if err == nil {
			h.currencyDeleted(writer, request, panel, code)
		} else {
			status, messageID := utils.CurrencyError(request, err)
			messages, flashErr := h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))

			if flashErr != nil {
				utils.FlashUnavailable(writer, request, flashErr)
			} else {
				dialog := pages.CurrencyDeleteDialog(request.Context(), request.URL.Path, string(code), messages)

				utils.RenderFragment(writer, request, status, components.ConfirmDelete(dialog))
			}
		}
	})
}

//
// Helpers
//

func (h *Handler) currencyForm(writer http.ResponseWriter, request *http.Request, edit bool) {
	h.withPanel(writer, request, layouts.SectionCurrencies, users.PermissionManageCurrency, func(request *http.Request, current *security.Identity, panel layouts.PanelProps) {
		var (
			currency *currencies.Currency
			err      error
		)

		if edit {
			currency, err = h.findCurrency(request)
		}

		if err != nil {
			h.renderCurrencyError(writer, request, current, err)
		} else {
			props := pages.CurrencyFormProps{
				Panel:  panel,
				Edit:   edit,
				Title:  admini18n.T(request.Context(), "currencies.create"),
				Action: panel.BasePath + "/currencies/create",
			}

			if edit {
				props.Code, props.Symbol = string(currency.Code), string(currency.Symbol)
				props.Title = admini18n.T(request.Context(), "currencies.edit_title", map[string]any{"Code": props.Code})
				props.Action = panel.BasePath + "/currencies/" + props.Code + "/edit"
			}

			status, responded := http.StatusOK, false

			if request.Method == http.MethodPost {
				var (
					form  utils.CurrencyForm
					saved *currencies.Currency
				)

				form, err = utils.ParseCurrencyForm(writer, request, edit)

				if err == nil {
					err = requestvalidation.Validate(request.Context(), form)
				}

				if err == nil && edit {
					saved, err = h.currencies.Update(request.Context(), input.UpdateCurrencyInput{Code: form.Code, Symbol: form.Symbol})
				} else if err == nil {
					saved, err = h.currencies.Create(request.Context(), input.CreateCurrencyInput{Code: form.Code, Symbol: form.Symbol})
				}

				// Editing keeps the persisted code, which the route identified.
				if !edit {
					props.Code = form.Code
				}

				props.Symbol = form.Symbol

				if err == nil {
					h.currencySaved(writer, request, panel, saved, edit)
					responded = true
				} else {
					var messageID string

					status, messageID = utils.CurrencyError(request, err)
					props.FieldErrors = utils.CurrencyFormFieldErrors(request.Context(), err)

					// Field errors stand beside their controls; any other failure
					// is shown at the top of the form.
					if len(props.FieldErrors) == 0 {
						var flashErr error

						props.Errors, flashErr = h.flashes.ShowError(writer, request, admini18n.T(request.Context(), messageID))

						if flashErr != nil {
							utils.FlashUnavailable(writer, request, flashErr)
							responded = true
						}
					}
				}
			}

			if !responded && request.Method == http.MethodPost {
				utils.RenderFormRegion(writer, request, status, pages.CurrencyFormID, pages.CurrencyFieldsForm(props))
			} else if !responded {
				h.render(writer, request, status, props.Title, pages.CurrencyForm(props))
			}
		}
	})
}

// A successful change is confirmed on the page the redirect leads to.

func (h *Handler) currencySaved(
	writer http.ResponseWriter,
	request *http.Request,
	panel layouts.PanelProps,
	currency *currencies.Currency,
	edit bool,
) {
	messageID := "currencies.created"

	if edit {
		messageID = "currencies.updated"
	}

	message := admini18n.T(request.Context(), messageID, map[string]any{"Code": string(currency.Code)})

	if err := h.flashes.Add(writer, request, flashmsg.Success, message); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		http.Redirect(writer, request, panel.BasePath+"/currencies/"+string(currency.Code)+"/view", http.StatusSeeOther)
	}
}

// A deletion is answered with the location to navigate to, because the
// request targeted the dialog rather than the page.

func (h *Handler) currencyDeleted(
	writer http.ResponseWriter,
	request *http.Request,
	panel layouts.PanelProps,
	code currencies.Code,
) {
	message := admini18n.T(request.Context(), "currencies.deleted", map[string]any{"Code": string(code)})
	location := panel.BasePath + "/currencies"

	if err := h.flashes.Add(writer, request, flashmsg.Success, message); err != nil {
		utils.FlashUnavailable(writer, request, err)
	} else {
		writer.Header().Set("HX-Location", utils.NavigationLocation(location))
		writer.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) findCurrency(request *http.Request) (*currencies.Currency, error) {
	var currency *currencies.Currency

	code, err := utils.ParseCurrencyCode(request)

	if err == nil {
		currency, err = h.currencies.FindByCode(request.Context(), string(code))
	}

	return currency, err
}

func (h *Handler) renderCurrencyError(
	writer http.ResponseWriter,
	request *http.Request,
	current *security.Identity,
	err error,
) {
	status, messageID := utils.CurrencyError(request, err)

	h.renderPanelError(writer, request, current, status, messageID)
}
