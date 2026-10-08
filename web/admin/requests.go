package admin

import (
	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/users"
	"github.com/radynsade/faryengo/middleware/requestvalidation"
)

func init() {
	// Domain mailbox syntax permits local domains that the built-in email tag
	// excludes. Delegate instead of defining another email parser.
	if err := requestvalidation.RegisterStringRule("mailbox", func(value string) bool {
		_, err := users.NewEmail(value)

		return err == nil
	}); err != nil {
		panic(err)
	}

	// Preserve the existing HTTP UUID encodings, including compact and URN forms.
	// Domain ID constructors separately reject zero identities.
	if err := requestvalidation.RegisterStringRule("uuid_input", func(value string) bool {
		_, err := uuid.Parse(value)

		return err == nil
	}); err != nil {
		panic(err)
	}
}

type signInRequest struct {
	Email    string `form:"email" validate:"required,mailbox,maxbytes=254"`
	Password string `form:"password" validate:"required,maxbytes=4096"`
}

type roleFormRequest struct {
	Name        map[string]string `form:"name" validate:"required,min=1,dive,keys,len=2,ascii,alpha,lowercase,endkeys,required,notblank,utf8"`
	Permissions []string          `form:"permissions" validate:"dive,oneof=manage_user view_user manage_role view_role"`
	IsSuper     string            `form:"is_super" validate:"omitempty,eq=1"`
}

type roleIDRequest struct {
	ID string `form:"role" validate:"required,uuid_input"`
}

type roleQueryRequest struct {
	IDLike      string   `form:"uuid" validate:"utf8,maxbytes=100,excludesall=0x00"`
	NameLike    string   `form:"name" validate:"utf8,maxbytes=500,excludesall=0x00"`
	Permissions []string `form:"permissions" validate:"dive,oneof=manage_user view_user manage_role view_role"`
	Super       string   `form:"super" validate:"omitempty,oneof=true false"`
	Sort        string   `form:"sort" validate:"oneof=uuid name super"`
	Order       string   `form:"order" validate:"omitempty,oneof=asc desc"`
	Page        int      `form:"page" validate:"min=1,max=1000000"`
	Size        int      `form:"size" validate:"min=1,max=100"`
	Language    string   `form:"language" validate:"len=2,ascii,alpha,lowercase"`
}

type roleDeleteRequest struct {
	Confirm string `form:"confirm" validate:"required,eq=delete"`
}
