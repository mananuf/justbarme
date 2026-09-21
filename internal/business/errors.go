package business

import "errors"

var (
	ErrBusinessNotFound = errors.New("business not found")
	// ErrUnsupportedImageFormat is returned when a logo upload's declared
	// or decoded content type isn't PNG, JPEG, or WebP.
	ErrUnsupportedImageFormat = errors.New("unsupported image format")
	// ErrImageTooLarge is returned when a logo upload exceeds the 2MB
	// limit. Checked against the raw upload size, before decoding.
	ErrImageTooLarge = errors.New("image exceeds the 2MB limit")
	// ErrInvalidImage is returned when the upload can't be decoded as an
	// image at all -- a corrupt file, or one that isn't actually image
	// data despite its declared content type.
	ErrInvalidImage = errors.New("could not decode image")
)
