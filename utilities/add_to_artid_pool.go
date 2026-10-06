package utilities

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	stateStructs "git.lunr.sh/luna/eurydice/state"
	"golang.org/x/image/draw"
)

func AddToArtIDPool(state *stateStructs.ApplicationState, imageBytes []byte) (string, error) {
	songArtID := ""

	imageData, _, err := image.Decode(bytes.NewReader(imageBytes))

	if err != nil {
		return "", fmt.Errorf("failed to decode embedded image: %w", err)
	}

	md5Hash := md5.New()

	if _, err = md5Hash.Write(imageBytes); err != nil {
		return "", fmt.Errorf("failed to hash embedded image: %w", err)
	}

	imageHash := md5Hash.Sum(nil)
	imageHashAsString := make([]byte, hex.EncodedLen(len(imageHash)))

	hex.Encode(imageHashAsString, imageHash)

	state.Logger.Debugf("Image hash: %s", string(imageHashAsString))
	songArtID = string(imageHashAsString)

	// TODO: this if statement is funky
	if _, err := os.Stat(filepath.Join(state.Config.AppStatePath, "thumbnails", string(imageHashAsString))); !os.IsExist(err) {
		// Downscale image and then write it
		newImage := image.NewRGBA(image.Rect(0, 0, 256, 256))
		draw.NearestNeighbor.Scale(newImage, newImage.Rect, imageData, imageData.Bounds(), draw.Over, nil)

		file, err := os.OpenFile(filepath.Join(state.Config.AppStatePath, "thumbnails", string(imageHashAsString)), os.O_WRONLY|os.O_CREATE, 0644)

		if err != nil {
			return "", fmt.Errorf("failed to open image (for writing) '%s': %w", string(imageHashAsString), err)
		}

		defer file.Close()

		err = jpeg.Encode(file, newImage, &jpeg.Options{
			Quality: 95,
		})

		if err != nil {
			return "", fmt.Errorf("failed to encode image '%s' as JPEG: %w", string(imageHashAsString), err)
		}
	}

	return songArtID, nil
}
