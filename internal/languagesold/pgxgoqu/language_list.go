package pgxgoqu

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"

	"github.com/radynsade/faryengo/internal/languages"
)

func (r *LanguageRepository) FindAll(ctx context.Context) ([]*languages.Language, error) {
	var result []*languages.Language
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else {
		query, args, buildErr := goqu.Dialect("postgres").From("language").
			Select("code", "english_name", "native_name", "is_fallback").Order(goqu.C("code").Asc()).Prepared(true).ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build list languages query: %w", buildErr)
		} else {
			rows, queryErr := r.db.Query(ctx, query, args...)

			if queryErr != nil {
				err = fmt.Errorf("list languages: %w", queryErr)
			} else {
				defer rows.Close()
				result = make([]*languages.Language, 0)

				for rows.Next() {
					var code, english, native string
					var fallback bool
					err = rows.Scan(&code, &english, &native, &fallback)

					if err == nil {
						var language *languages.Language
						language, err = languages.NewLanguage(languages.LanguageCode(code), languages.LanguageEnglishName(english), languages.LanguageNativeName(native), fallback)

						if err == nil {
							result = append(result, language)
						}
					}

					if err != nil {
						break
					}
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("read languages: %w", err)
					result = nil
				}
			}
		}
	}

	return result, err
}
