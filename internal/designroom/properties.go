package designroom

import (
	"strings"
)

// editedProperties are the inline style properties the properties panel writes
// besides the four a drag writes: the ones people change at once in different
// places, so each is its own entry in the room. The list is
// web/src/design/properties/owned.json, which the browser and the frames'
// inspector read too; a test holds the three together. `display` is not here:
// the outline's hide writes it, and an element whose own markup says
// display:none must stay hidden until the outline says otherwise.
const editedProperties = `
position flex-wrap flex-direction align-items justify-content gap flex-grow
padding padding-top padding-right padding-bottom padding-left
margin margin-top margin-right margin-bottom margin-left
border-radius border-top-left-radius border-top-right-radius border-bottom-right-radius border-bottom-left-radius
border-top-width border-top-style border-top-color
border-right-width border-right-style border-right-color
border-bottom-width border-bottom-style border-bottom-color
border-left-width border-left-style border-left-color
border-width border-style border-color background-color opacity box-shadow
font-family font-size font-weight font-style text-align line-height letter-spacing color
background-image background-size background-position background-repeat object-fit`

func ownedSet() map[string]bool {
	set := map[string]bool{"left": true, "top": true, "width": true, "height": true}
	for _, p := range strings.Fields(editedProperties) {
		set[p] = true
	}
	return set
}
