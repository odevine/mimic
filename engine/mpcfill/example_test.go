package mpcfill_test

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/odevine/mimic/engine/mpcfill"
)

// render stands in for rendering a card: it writes a blank PNG at MPC
// Autofill's 300 DPI convention, where a card with bleed is 1110 pixels tall
func render(dir, file string) error {
	name := filepath.Join(dir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	if err := png.Encode(f, image.NewGray(image.Rect(0, 0, 816, 1110))); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// A project with a shared cardback and one double-faced card
func ExamplePlan() {
	dir, _ := os.MkdirTemp("", "mpcfill")
	defer os.RemoveAll(dir)

	layout, err := mpcfill.Plan(mpcfill.Project{
		Stock:    mpcfill.S30,
		Cardback: &mpcfill.Face{Name: "House Back"},
		Cards: []mpcfill.Card{
			{Front: mpcfill.Face{Name: "Island"}, Quantity: 4},
			{Front: mpcfill.Face{Name: "Delver of Secrets"}, Back: &mpcfill.Face{Name: "Insectile Aberration"}},
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	files := []string{layout.Cardback}
	for _, c := range layout.Cards {
		files = append(files, c.Front)
		if c.Back != "" {
			files = append(files, c.Back)
		}
	}
	for _, f := range files {
		fmt.Println(f)
		if err := render(dir, f); err != nil {
			fmt.Println(err)
			return
		}
	}

	if err := mpcfill.WriteOrder(dir, layout); err != nil {
		fmt.Println(err)
		return
	}
	order, _ := os.ReadFile(filepath.Join(dir, mpcfill.OrderFile))
	fmt.Print(string(order))
	// Output:
	// cardback/House Back.png
	// fronts/Island.png
	// fronts/Delver of Secrets.png
	// backs/Insectile Aberration.png
	// <?xml version="1.0" encoding="UTF-8"?>
	// <order>
	//     <details>
	//         <quantity>5</quantity>
	//         <stock>(S30) Standard Smooth</stock>
	//         <foil>false</foil>
	//     </details>
	//     <fronts>
	//         <card>
	//             <id>./fronts/Island.png</id>
	//             <sourceType>Local File</sourceType>
	//             <slots>0,1,2,3</slots>
	//             <name>Island.png</name>
	//             <query>island</query>
	//         </card>
	//         <card>
	//             <id>./fronts/Delver of Secrets.png</id>
	//             <sourceType>Local File</sourceType>
	//             <slots>4</slots>
	//             <name>Delver of Secrets.png</name>
	//             <query>delver of secrets</query>
	//         </card>
	//     </fronts>
	//     <backs>
	//         <card>
	//             <id>./backs/Insectile Aberration.png</id>
	//             <sourceType>Local File</sourceType>
	//             <slots>4</slots>
	//             <name>Insectile Aberration.png</name>
	//             <query>insectile aberration</query>
	//         </card>
	//     </backs>
	//     <cardback>./cardback/House Back.png</cardback>
	// </order>
}

// Plan checks the whole project before anything is rendered, and each
// validation failure wraps one of the package's sentinel errors
func ExamplePlan_validation() {
	_, err := mpcfill.Plan(mpcfill.Project{
		Stock: mpcfill.P10,
		Foil:  true,
		Cards: []mpcfill.Card{{Front: mpcfill.Face{Name: "A"}}},
	})
	fmt.Println(err)
	// Output:
	// mpcfill: cardstock does not support foil: (P10) Plastic
}
