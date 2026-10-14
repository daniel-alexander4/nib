/*
Copyright 2020 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pdfcpu

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PropertiesList returns a list of document properties as recorded in the document info dict.
func PropertiesList(ctx *model.Context) ([]string, error) {
	list := make([]string, 0, len(ctx.Properties))
	keys := make([]string, len(ctx.Properties))
	i := 0
	for k := range ctx.Properties {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := ctx.Properties[k]
		list = append(list, fmt.Sprintf("%s = %s", k, v))
	}
	return list, nil
}

// PropertiesAdd adds properties into the document info dict.
// Returns true if at least one property was added.
func PropertiesAdd(ctx *model.Context, properties map[string]string) error {
	if err := ensureInfoDictAndFileID(ctx); err != nil {
		return err
	}

	d, _ := ctx.DereferenceDict(*ctx.Info)

	for k, v := range properties {
		s, err := types.EscapedUTF16String(v)
		if err != nil {
			return err
		}
		d[k] = types.StringLiteral(*s)
		ctx.Properties[k] = *s
	}

	return nil
}

// PropertiesRemove deletes specified document properties.
// If properties is empty, it removes all properties and catalog XMP metadata.
// Returns true if at least one property was removed.
func PropertiesRemove(ctx *model.Context, properties []string) (bool, error) {
	if len(properties) == 0 {
		return removeAllProperties(ctx)
	}

	if ctx.Info == nil {
		return false, nil
	}

	d, err := ctx.DereferenceDict(*ctx.Info)
	if err != nil || d == nil {
		return false, err
	}

	var removed bool
	for _, k := range properties {
		_, ok := d[k]
		if ok && !removed {
			delete(d, k)
			delete(ctx.Properties, k)
			removed = true
		}
	}

	return removed, nil
}

func removeAllProperties(ctx *model.Context) (bool, error) {
	var removed bool

	if ctx.Info != nil {
		d, err := ctx.DereferenceDict(*ctx.Info)
		if err != nil || d == nil {
			return false, err
		}
		for k := range ctx.Properties {
			delete(d, types.EncodeName(k))
			removed = true
		}
		ctx.Properties = map[string]string{}
	}

	rootDict, err := ctx.Catalog()
	if err != nil {
		return removed, err
	}
	if _, ok := rootDict["Metadata"]; ok {
		delete(rootDict, "Metadata")
		ctx.CatalogXMPMeta = nil
		removed = true
	}

	return removed, nil
}
