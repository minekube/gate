package ping

import (
	"encoding/json"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestServerPing_JSON(t *testing.T) {
	// Test json marshalling and unmarshalling of ServerPing.
	p := &ServerPing{
		Version: Version{
			Protocol: 1,
			Name:     "1",
		},
		Players: &Players{
			Online: 1,
			Max:    1,
			Sample: []SamplePlayer{
				{
					Name: "1",
					ID:   uuid.New(),
				},
			},
		},
		Description: &component.Text{
			Content: "Hello",
		},
		Favicon: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAYAAACqaXHeAAAABGdBTUEAALGPC/xhBQAAACBjSFJNAAB6JgAAgIQAAPoAAACA6AAAdTAAAOpgAAA6mAAAF3CculE8AAAABmJLR0QA/wD/AP+gvaeTAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAB3RJTUUH5AgJCgs6JBZy0AAAB+lJREFUeNrtmGuMXVUZht/LOvcz7diWklbJaKGCWKXFUiiBQjAEhRSLSCEqIKAiogRTmiriBeSiUUhjMCZe2qQxNE0EUZSCidAgYMBSLBJCDC1taAg4xaq1cz/788feZzqgocCoP2Q/f87JOXvvfOtd73dZGygpKSkpKSkpKSkpKSkpKSkpKSkpKXnzwNd7w+ULD0NEBtmQBFqQBNuICFRYwzc3bf7/E+Cyo+cgAIiEbESWJdl1WSQ5VGvWRwnk/0XgpnvfmAhrv3h+HhgFFeJCBCLw8Wt//B8XIB3ogs8umJMHAOCQvtnYtfP5eRGxVNaxMmdKgqzdWSfbIuuuocHBLa2ednx16WJcd9fvXn9EAQSQyGgBwUCMMbAvAvHfcIAOaBHnl0REY9fO56+itFHSjbI/JHuxrMWyl8r6mq27m+3WKpJ1kvjG2UtevyUtyDqK0t2UNklaLalu63+fAp9bNBdZFogsq0j6OsVVkix7L8WNkh6VLVuLlXyapKbsMUkrR4aGV9caNbRnTkWKPC0kgdpv7e7n2GgH7ant8WgiYomoX8uqUXoIwKm1Rn0wJQMAGu0mBv8xhEZPAxIhOX9W8byR4VHUGjUcf86qyaVApVrB6MgYIC4leaUsS/oLySsprQcwBgRkVW1fIfsGWVVJn29Naf8CwPYUedAjQ8OsNxuHApiHwAwAIwB2AtjaaNX/CgQAiuQM27NIhixQqqaU+mTtA9AfEUMA0JzSRERMB3BUIPoCgQjsiIg/NHuaewDg0TtvxqJlK964A6447nAE0CLwM0mnygbFGzujY19WMhD5bjgZTu6hdLekE4qduDAi1jWntIEseimuIHmBrLfJVuGAAdmbZV1t6yGnNI3kBkkLaE2zRNnDsnbLGiR1EcUHK5WKlbyc5BdkvUdSPXeAB21tln3D6PDIvdV6DQBeVYRXdYDyvHsXyYW5dd1PYoNURafTwS2/fRIAsPKU+eg96C17EVhNcavskPgCQCDCtK4RuaLY0WdlPWW7T9a7JS+RdYukpbJHip3PcoHctXUmKyMZkuBK+jTJb8tqSeqn9ICthuyFsk4kubZar50P4DeTSgHZAHAEyd6i7z9LYgcA9M6chuuXnwzLoPLWiIjbKd7ezfV8B+JIkp+gVNzPj0jaKvsQ2xtkLZI1n+TRo8Mj99QatY85+WRJ62TXJW0leb6kfZ2xzp/rzcZ8ktcUi98B4hJZD1KqyLqU5E0AZgH4EoDfA/j7GxbAuc0PpshCgJcoDiHGOxIDMZfgdACh5InFbRuJflA1kffJNsVNtXptS7GzOyJii6RFsqqkZjsZ1XqtX/aLJLPiOcOSnsuybLA1tQ2S51CcXQi/RvZ9TgaBEZA/BHAugEUAjgMwH8ADkxAgARGaULlJERGAJESWWdK1kpZJHJMUyncaFD+TZdltTumxl17oP3f2nEN6Jc0DeSmIWQAato/tVm5KjbzwVos5iONiklSlVoHtOsljKMFWJvswWSsmtHMCqBffWwCOnJwANgKxp7soWdNJNYAYcTKyTLBlSklWWK7ISnnQqgDAvMXz4pmt2z4gcRWlYwrrTsxvOC+uRBSuA0AS+8UhUqUC2XWK04tYJOmCA6R476RqgJMRgW0SB2Q1Zb+dZB+JJyQByDqUrpP0fVGZk1fSOsO5AJCNbX/cfoKsNZJmSRqguE7SJol7bH9K1umyUaz/XwSgVIzfgpMySmOSQHIUwG1FK504JXVQ9NQD7f5rLILxlKRtebvxQbLOlvWELIyOjIbkJ11JqNaqMxAxOz8cGRLzRUgflTSrsPIaklcC6NgJqZJOG0+viQIExgsrRZAqnuUBWbuKHBeAXwL46cSeHnnezwCQAXh6UqNwqiTUW80XndJ6O3X7/WVO6RxSmjKtF+3eNqq16hSSVyt5vu3udXjh2V1w8ludjPz3tKNSq3ZaU3tQrVdnyn6fnf+n4h6Nf0/dexq2VKlVIWsMwKauQQGcPSHnEcA7AawHcA+ANQAOnpQDsixDZBlk/Uj2SbZOk3WQ5B/IOhOIxyNQp3iKpJOK9naErDpJ9B15GGxvL4oWZF+i5L0A/kZruaT3jhc6KSGimwIDpMaK8fZwSStJPgLgfgB3ALgAwEIAZwEYALARQA+ACwEcUYR/RwB/4mTOAgBw43nvR6okSJoj61uyz7RV3T/Pu7uAp2ytln2LrDapiyWudUoLZK2XfbgnnAEo7bb9sKwzC6tfjyy+Um81EMAMST+XdXxeawAAzwM4EcB2AMcDuBXAgn8T8kjhgpUA+ic1CeaFMN89kNudfBGlU2V9UPZcWQ1JeyU9RvInTmk3xdmSWpSeQAC2Hpd9nqxPyjpKsmTtJLlByS+KfFqSKW4OZHAlgcBuAJdTugTAoUWcOwDsLcJ6GMAyAMsLUWYCGAWwragLGwtnYNIOAICbLz4DKe0/0R13+hI8fv+jDVlJ0miWZUOykZLHT3sUQRAUUa3X8OGrvotf3bqyRYlOaSCyLJvoIjIPpd5uoG/uO/DcMzu743i1iHOsk3U6BMffPnXPbEUdyCJigOTL3htM6jD0Sr53+VmQ07gQ432aQiBQq9cAomhdQgBo9bQg7x+eim6AyDJUm00gAikRlLCvswc9noZqT6uozvGyELvvRI5ddhUeufM7ebcgX/k+BXwNCy8pKSkpKSkpKSkpKSkpKSkpKSkpKXkz8k8RHxEbZN/8lgAAACV0RVh0ZGF0ZTpjcmVhdGUAMjAyMC0wOC0wOVQxMDoxMTo0MyswMDowMN6nNEYAAAAldEVYdGRhdGU6bW9kaWZ5ADIwMjAtMDgtMDlUMTA6MTE6NDMrMDA6MDCv+oz6AAAAAElFTkSuQmCC",
	}

	// Marshal to json.
	b, err := json.Marshal(p)
	require.NoError(t, err)

	// Unmarshal from json.
	var p2 ServerPing
	err = json.Unmarshal(b, &p2)
	require.NoError(t, err)

	// Compare.
	require.Equal(t, p, &p2)
}

func TestServerPing_UnmarshalJSON_description_string(t *testing.T) {
	const jsonStr = `{
"description": "                §eGate Proxy\n             §6§lHello World"
}`

	var p ServerPing
	err := json.Unmarshal([]byte(jsonStr), &p)
	require.NoError(t, err)

	dj, err := util.Marshal(-1, p.Description)
	require.NoError(t, err)

	// need this step for ordering of json keys
	m := map[string]any{}
	err = json.Unmarshal(dj, &m)
	require.NoError(t, err)
	js, err := json.Marshal(&m)
	require.NoError(t, err)

	const exp = `{"extra":[{"color":"yellow","text":"Gate Proxy\n             "},{"color":"gold","extra":[{"bold":true,"text":"Hello World"}],"text":""}],"text":"                "}`
	require.Equal(t, exp, string(js))
}
