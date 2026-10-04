package pgxgoqu

import (
	"context"
	"fmt"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/radynsade/faryengo/internal/languages"
	"github.com/radynsade/faryengo/internal/security"
)

func roleFilterQuery(filters security.RoleFilters) *goqu.SelectDataset {
	query := goqu.Dialect("postgres").From("role")

	if filters.IDLike != "" {
		query = query.Where(goqu.L(`"role".id::text ILIKE ?`, likeSubstring(filters.IDLike)))
	}

	if filters.NameLike != "" {
		query = query.Where(goqu.L(`EXISTS (SELECT 1 FROM "translation" WHERE text_id = "role".name_id AND content ILIKE ?)`, likeSubstring(filters.NameLike)))
	}

	if len(filters.Permissions) > 0 {
		query = query.Where(goqu.Or(goqu.Ex{"is_super": true}, goqu.L(`"role".permissions @> ?`, rolePermissionsArray(filters.Permissions))))
	}

	if filters.IsSuper != nil {
		query = query.Where(goqu.Ex{"is_super": *filters.IsSuper})
	}

	return query
}

func likeSubstring(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}

func (r *RoleRepository) Count(ctx context.Context, filters security.RoleFilters) (int, error) {
	var total int
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if filterErr := filters.Validate(); filterErr != nil {
		err = filterErr
	} else {
		query, args, buildErr := roleFilterQuery(filters).Select(goqu.COUNT("*")).Prepared(true).ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build count roles query: %w", buildErr)
		} else if scanErr := r.db.QueryRow(ctx, query, args...).Scan(&total); scanErr != nil {
			err = fmt.Errorf("count roles: %w", scanErr)
		}
	}

	return total, err
}

func (r *RoleRepository) Find(ctx context.Context, options security.RoleQuery) ([]*security.Role, error) {
	var roles []*security.Role
	var err error

	if r == nil || r.db == nil {
		err = ErrNilPool
	} else if validationErr := options.Validate(); validationErr != nil {
		err = validationErr
	} else {
		query := roleFilterQuery(options.Filters).Select("id", goqu.L(`"permissions"::text[]`),
			goqu.L(`ARRAY(SELECT language_code FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`),
			goqu.L(`ARRAY(SELECT content FROM "translation" WHERE text_id = "role".name_id ORDER BY language_code)`), "is_super")
		var sort exp.Orderable = goqu.C("id")

		switch options.Sort {
		case security.RoleSortName:
			sort = goqu.L(`lower((SELECT content FROM "translation" t JOIN "language" l ON l.code = t.language_code WHERE t.text_id = "role".name_id ORDER BY (t.language_code = ?) DESC, l.is_fallback DESC, t.language_code LIMIT 1))`, string(options.Language))
		case security.RoleSortSuper:
			sort = goqu.C("is_super")
		}

		order := sort.Asc()

		if options.Descending {
			order = sort.Desc()
		}

		query = query.Order(order)

		if options.Sort != security.RoleSortID {
			query = query.OrderAppend(goqu.C("id").Asc())
		}

		sql, args, buildErr := query.Limit(uint(options.PageSize)).Offset(uint((options.Page - 1) * options.PageSize)).Prepared(true).ToSQL()

		if buildErr != nil {
			err = fmt.Errorf("build find roles query: %w", buildErr)
		} else {
			var rows pgx.Rows
			rows, err = r.db.Query(ctx, sql, args...)

			if err != nil {
				err = fmt.Errorf("find roles: %w", err)
			} else {
				defer rows.Close()
				roles = make([]*security.Role, 0, options.PageSize)

				for rows.Next() {
					var role *security.Role
					role, err = scanRole(rows)

					if err != nil {
						break
					}

					roles = append(roles, role)
				}

				if err == nil {
					err = rows.Err()
				}

				if err != nil {
					err = fmt.Errorf("read roles: %w", err)
				}
			}
		}
	}

	if err != nil {
		roles = nil
	}

	return roles, err
}

func scanRole(row pgx.Row) (*security.Role, error) {
	var role *security.Role
	var id pgtype.UUID
	var permissions, codes, contents []string
	var isSuper bool
	err := row.Scan(&id, &permissions, &codes, &contents, &isSuper)

	if err == nil {
		if len(codes) != len(contents) {
			err = fmt.Errorf("decode role: mismatched name translations")
		} else {
			translations := make([]languages.Translation, 0, len(codes))

			for index, code := range codes {
				var translation languages.Translation
				translation, err = languages.NewTranslation(languages.LanguageCode(code), contents[index])

				if err != nil {
					break
				}

				translations = append(translations, translation)
			}

			if err == nil {
				var name languages.Text
				name, err = languages.NewText(translations)
				rawPermissions := make([]security.Permission, len(permissions))

				for index, permission := range permissions {
					rawPermissions[index] = security.Permission(permission)
				}

				if err == nil {
					role, err = security.NewRole(security.RoleID(id.Bytes), name, rawPermissions)
				}

				if err == nil {
					role.SetIsSuper(isSuper)
				}
			}
		}
	}

	return role, err
}
