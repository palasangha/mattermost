package plugin

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pkg/errors"
	"golang.org/x/text/language"
)

func (p *HelpersImpl) Loadi18nBundle() (*i18n.Bundle, error) {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bundle path")
	}

	i18nDir := filepath.Join(bundlePath, "assets", "i18n")
	files, err := ioutil.ReadDir(i18nDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open i18n directory")
	}

	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "active.") {
			continue
		}

		if file.Name() == "active.en.json" {
			continue
		}
		_, err = bundle.LoadMessageFile(filepath.Join(i18nDir, file.Name()))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load message file %s", file.Name())
		}
	}

	return bundle, nil
}
