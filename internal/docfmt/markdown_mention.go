package docfmt

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// mention reads [@Name](mention:account) as a mention of that person.
func (c *converter) mention(n *ast.Link, user string, emit func(Inline)) error {
	name, ok := strings.CutPrefix(c.plain(n), "@")
	if !ok {
		return c.refuse(n, "mention of %q: the text of a mention link is @ and the name, such as [@Anna Lee](mention:anna.lee)", user)
	}
	if err := mentionUser(user, "the account"); err != nil {
		return c.refuse(n, "mention of %q: %s", user, strings.TrimPrefix(err.Error(), "the account: "))
	}
	if err := mentionName(name, "the name"); err != nil {
		return c.refuse(n, "mention of %q: %s", user, strings.TrimPrefix(err.Error(), "the name: "))
	}
	emit(Inline{User: user, Name: name})
	return nil
}

func (c *converter) link(n *ast.Link, marks []string, href string, emit func(Inline), image func(*ast.Image)) error {
	if strings.HasPrefix(href, MentionScheme) && !c.legacy {
		return c.mention(n, strings.TrimPrefix(href, MentionScheme), emit)
	}
	if err := linkTarget(href); err != nil {
		if c.legacy {
			return c.walkInlines(n, marks, "", emit, image)
		}
		return c.refuse(n, "link %q: %s", href, err)
	}
	return c.walkInlines(n, with(marks, MarkLink), href, emit, image)
}
