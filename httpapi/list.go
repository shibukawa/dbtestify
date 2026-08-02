package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
)

type ListResult struct {
	DataSets []string `json:"datasets"`
}

func dumpDataSetList(useJson bool, w io.Writer, root fs.FS, baseURL string) {
	if useJson {
		result := ListResult{
			DataSets: ListDataSets(root),
		}
		e := json.NewEncoder(w)
		e.Encode(&result)
	} else {
		for _, ds := range ListDataSets(root) {
			fmt.Fprintf(w, "* %s\n", ds)
			fmt.Fprintf(w, "    * Seed:   curl -X POST %s/api/seed/%s\n", baseURL, ds)
			fmt.Fprintf(w, "    * Assert: curl %s/api/assert/%s\n", baseURL, ds)
		}
	}
}

// ListDataSets returns the YAML dataset files under root, in walk order.
//
// The paths it returns are the ones the seed and assert endpoints accept.
func ListDataSets(root fs.FS) []string {
	var result []string
	fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		ext := path.Ext(p)
		if !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
			result = append(result, p)
		}
		return nil
	})
	return result
}
