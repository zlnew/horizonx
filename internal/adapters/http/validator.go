package http

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

func ValidateStruct(payload any) map[string]string {
	err := validate.Struct(payload)
	if err == nil {
		return nil
	}

	errors := make(map[string]string)

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, error := range validationErrors {
			fieldName := strings.ToLower(error.Field())
			switch error.Tag() {
			case "required":
				errors[fieldName] = fmt.Sprintf("The %s field is required.", error.Field())
			case "email":
				errors[fieldName] = fmt.Sprintf("The %s must be a valid email address.", error.Field())
			case "min":
				errors[fieldName] = fmt.Sprintf("The %s must be at least %s characters.", error.Field(), error.Param())
			case "eqfield":
				errors[fieldName] = fmt.Sprintf("The %s field must be equal to %s field.", error.Field(), error.Param())
			default:
				errors[fieldName] = fmt.Sprintf("The %s field is invalid.", error.Field())
			}
		}
	}

	return errors
}
