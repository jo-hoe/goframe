package common

import (
	"fmt"
	"net/http"

	"github.com/go-playground/validator"
	"github.com/labstack/echo/v4"
)

type GenericEchoValidator struct {
	Validator *validator.Validate
}

func (gv *GenericEchoValidator) Validate(i interface{}) error {
	if gv.Validator == nil {
		gv.Validator = validator.New()
	}
	if err := gv.Validator.Struct(i); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("received invalid request body: %v", err))
	}
	return nil
}
