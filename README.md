
	imageUrl := fmt.Sprintf("/image.%s", s.imageTargetType)
	e.GET(imageUrl, func(c echo.Context) error {
		return c.String(200, "Image processing request received")
	})