package packet

import (
	"errors"
	"fmt"
	"github.com/gammazero/deque"
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/brigadier"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

const (
	NodeTypeRoot     byte = 0x00
	NodeTypeLiteral  byte = 0x01
	NodeTypeArgument byte = 0x02

	FlagNodeType       byte = 0x03
	FlagExecutable     byte = 0x04
	FlagIsRedirect     byte = 0x08
	FlagHasSuggestions byte = 0x10
)

var PlaceholderCommand = brigodier.CommandFunc(func(c *brigodier.CommandContext) error { return nil })

type AvailableCommands struct {
	RootNode *brigodier.RootCommandNode
}

var _ proto.Packet = (*AvailableCommands)(nil)

func (a *AvailableCommands) Encode(_ *proto.PacketContext, wr io.Writer) (err error) {
	// Assign all the children an index.
	var childrenQueue deque.Deque
	childrenQueue.PushFront(a.RootNode)
	idMappings := map[brigodier.CommandNode]int{}
	for childrenQueue.Len() != 0 {
		child := childrenQueue.PopFront().(brigodier.CommandNode)
		if _, ok := idMappings[child]; !ok {
			idMappings[child] = len(idMappings)
			child.ChildrenOrdered().Range(func(_ string, grantChild brigodier.CommandNode) bool {
				childrenQueue.PushBack(grantChild)
				return true
			})
		}
	}

	// Now serialize the children.
	err = util.WriteVarInt(wr, len(idMappings))
	if err != nil {
		return err
	}
	for child := range idMappings {
		err = encodeNode(wr, child, idMappings)
		if err != nil {
			return err
		}
	}
	return util.WriteVarInt(wr, idMappings[a.RootNode])
}

func encodeNode(wr io.Writer, node brigodier.CommandNode, idMappings map[brigodier.CommandNode]int) error {
	var flags byte
	if node.Redirect() != nil {
		flags |= FlagIsRedirect
	}
	if node.Command() != nil {
		flags |= FlagExecutable
	}

	switch n := node.(type) {
	case *brigodier.LiteralCommandNode:
		flags |= NodeTypeLiteral
	case *brigodier.ArgumentCommandNode:
		flags |= NodeTypeArgument
		if n.CustomSuggestions() != nil {
			flags |= FlagHasSuggestions
		}
	case *brigodier.RootCommandNode:
	default:
		return fmt.Errorf("unknown node type %T", node)
	}

	err := util.WriteByte(wr, flags)
	if err != nil {
		return err
	}
	err = util.WriteVarInt(wr, len(node.Children()))
	if err != nil {
		return err
	}
	node.ChildrenOrdered().Range(func(_ string, child brigodier.CommandNode) bool {
		err = util.WriteVarInt(wr, idMappings[child])
		return err == nil
	})
	if err != nil {
		return err
	}
	if node.Redirect() != nil {
		err = util.WriteVarInt(wr, idMappings[node.Redirect()])
		if err != nil {
			return err
		}
	}

	err = util.WriteString(wr, node.Name())
	if err != nil {
		return err
	}
	if n, ok := node.(*brigodier.ArgumentCommandNode); ok {
		err = brigadier.Encode(wr, n.Type())
		if err != nil {
			return err
		}

		if provider := n.CustomSuggestions(); provider != nil {
			name := "minecraft:ask_server"
			if p, ok := provider.(*protocolSuggestionProvider); ok {
				name = p.name
			}
			err = util.WriteString(wr, name)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *AvailableCommands) Decode(_ *proto.PacketContext, rd io.Reader) error {
	commands, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	wireNodes := make([]*WireNode, commands)
	for i := 0; i < commands; i++ {
		wn := &WireNode{IDx: i}
		if err = wn.decode(rd); err != nil {
			return err
		}
		wireNodes[i] = wn
	}

	var ok bool
	queue := append([]*WireNode{}, wireNodes...) // copy
	// Iterate over the deserialized nodes and attempt to form a graph.
	// We also resolve any cycles that exist.
	for len(queue) != 0 {
		var cycling bool

		for i := 0; i < len(queue); {
			node := queue[i]
			ok, err = node.toNodes(wireNodes)
			if err != nil {
				return err
			}
			if ok {
				cycling = true
				queue = removeWN(queue, i)
				// don't increment i since removing element at i
				// makes i now point to the next element
				continue
			}
			i++
		}

		if !cycling {
			// Uh-oh. We can't cycle. This is bad.
			return errors.New("stopped cycling; the root node can't be built")
		}
	}

	rootIDx, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	built := wireNodes[rootIDx].Built
	a.RootNode, ok = built.(*brigodier.RootCommandNode)
	if !ok {
		return fmt.Errorf("built node type is not *RootCommandNode (%T)", built)
	}
	return nil
}

// remove element from slice: order is not important
func removeWN(s []*WireNode, i int) []*WireNode {
	s[len(s)-1], s[i] = s[i], s[len(s)-1]
	return s[:len(s)-1]
}

type WireNode struct {
	IDx        int
	Flags      byte
	Children   []int
	RedirectTo int
	Args       brigodier.NodeBuilder // nil-able
	Built      brigodier.CommandNode
	Validated  bool
}

func (w *WireNode) decode(rd io.Reader) (err error) {
	w.Flags, err = util.ReadByte(rd)
	if err != nil {
		return err
	}
	w.Children, err = util.ReadIntArray(rd)
	if err != nil {
		return err
	}
	w.RedirectTo = -1
	if w.Flags&FlagIsRedirect > 0 {
		w.RedirectTo, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	switch t := w.Flags & FlagNodeType; t {
	case NodeTypeRoot:
	case NodeTypeLiteral:
		literal, err := util.ReadString(rd)
		if err != nil {
			return err
		}
		w.Args = brigodier.Literal(literal).NodeBuilder()
	case NodeTypeArgument:
		name, err := util.ReadString(rd)
		if err != nil {
			return err
		}
		argType, err := brigadier.Decode(rd)
		if err != nil {
			return err
		}
		argBuilder := brigodier.Argument(name, argType)
		if w.Flags&FlagHasSuggestions != 0 {
			name, err = util.ReadString(rd) // name not needed
			if err != nil {
				return err
			}
			argBuilder.Suggests(&protocolSuggestionProvider{name: name})
		}
		w.Args = argBuilder.NodeBuilder()
	default:
		return fmt.Errorf("unknown node type %d", t)
	}
	return nil
}

func (w *WireNode) toNodes(wireNodes []*WireNode) (bool, error) {
	if !w.Validated {
		if err := w.validate(wireNodes); err != nil {
			return false, err
		}
	}

	if w.Built == nil {
		nodeType := w.Flags & FlagNodeType
		if nodeType == NodeTypeRoot {
			w.Built = &brigodier.RootCommandNode{}
		} else {
			if w.Args == nil {
				return false, errors.New("non-root node without args builder")
			}

			// Add any redirects
			if w.RedirectTo != -1 {
				redirect := wireNodes[w.RedirectTo]
				if redirect.Built != nil {
					w.Args.Redirect(redirect.Built)
				} else {
					// Redirect node does not yet exist
					return false, nil
				}
			}

			// If executable, add an empty command
			if w.Flags&FlagExecutable != 0 {
				w.Args.Executes(PlaceholderCommand)
			}

			w.Built = w.Args.Build()
		}
	}

	for _, child := range w.Children {
		if wireNodes[child].Built == nil {
			// The child is not yet decoded. The node can't be built now.
			return false, nil
		}
	}

	// Associate children with nodes
	for _, child := range w.Children {
		childNode := wireNodes[child].Built
		if _, ok := childNode.(*brigodier.RootCommandNode); !ok {
			w.Built.AddChild(childNode)
		}
	}

	return true, nil
}

func (w *WireNode) validate(wireNodes []*WireNode) error {
	// Ensure all children exist.
	// Note that we delay checking if the node has been built yet;
	// that needs to come after this node is built.
	for _, child := range w.Children {
		if child < 0 || child >= len(wireNodes) {
			return fmt.Errorf("node points to non-existent index %d", child)
		}
	}
	if w.RedirectTo != -1 {
		if w.RedirectTo < 0 || w.RedirectTo >= len(wireNodes) {
			return fmt.Errorf("redirect node points to non-existent index %d", w.RedirectTo)
		}
	}
	w.Validated = true
	return nil
}

// protocolSuggestionProvider is a placeholder brigodier.SuggestionProvider
// used internally to preserve the suggestion provider name.
type protocolSuggestionProvider struct{ name string }

var _ brigodier.SuggestionProvider = (*protocolSuggestionProvider)(nil)

func (p *protocolSuggestionProvider) Suggestions(
	_ *brigodier.CommandContext,
	b *brigodier.SuggestionsBuilder,
) *brigodier.Suggestions {
	return b.Build()
}
