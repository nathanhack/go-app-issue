package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/maxence-charriere/go-app/v9/pkg/app"
	svg "github.com/nathanhack/go-app-svg"
	"github.com/nathanhack/go-app-svg/attr"
	"golang.org/x/exp/slices"
)

type NodeType string

const (
	CheckNode NodeType = "CheckNode"
	VarNode   NodeType = "VarNode"
	Ctrl      int      = 17
	KeyZ      int      = 90
	Esc       int      = 27
	Del       int      = 46
)

type Node struct {
	app.Compo
	parent      *svgCanvas
	NodeType    NodeType
	Index       int
	Connections map[int]*Node
	X, Y        int
	Selected    bool
	Highlighted bool
}

func (n *Node) Scale() int {
	return n.parent.Scale
}

func (n *Node) Render() app.UI {
	s := n.Scale()
	if n.NodeType == CheckNode {
		return svg.Rect(attr.Width(s*2), attr.Height(s*2),
			attr.X(scaleUp(n.X, s)-s), attr.Y(scaleUp(n.Y, s)-s),
			attr.If(n.Selected, attr.Stroke("red"), attr.Stroke("rgb(0,0,0)")),
			attr.If(n.Highlighted, attr.Fill("lightblue"), attr.Fill("blue")),
			attr.If(n.Selected, attr.StrokeWidth(4), attr.StrokeWidth(1)),
		).OnClick(n.OnClick).OnMouseEnter(n.OnMouseEnter).OnMouseLeave(n.OnMouseLeave)
	}

	return svg.Circle(attr.R(s), attr.Cx(scaleUp(n.X, s)), attr.Cy(scaleUp(n.Y, s)),
		attr.If(n.Selected, attr.Stroke("red"), attr.Stroke("rgb(0,0,0)")),
		attr.If(n.Highlighted, attr.Fill("lightblue"), attr.Fill("blue")),
		attr.If(n.Selected, attr.StrokeWidth(4), attr.StrokeWidth(1)),
	).OnClick(n.OnClick).OnMouseEnter(n.OnMouseEnter).OnMouseLeave(n.OnMouseLeave)
}

func (n *Node) RenderLines() app.UI {
	if n.NodeType == CheckNode && len(n.Connections) > 0 {
		s := n.Scale()
		elem := make([]any, 0, 1+len(n.Connections))

		for _, node := range n.Connections {
			elem = append(elem, svg.Line(
				attr.X1(scaleUp(n.X, s)), attr.Y1(scaleUp(n.Y, s)),
				attr.X2(scaleUp(node.X, s)), attr.Y2(scaleUp(node.Y, s)),
				attr.Stroke("black"), attr.StrokeWidth("2"),
			))
		}

		return svg.Svg(elem...)
	}
	return svg.Svg()
}

func (n *Node) ConnectTo(node *Node) bool {
	if node == nil {
		return false
	}
	if n.NodeType == node.NodeType {
		panic(fmt.Sprintf("can't connect same node types: %T", n.NodeType))
	}

	if n.NodeType != CheckNode {
		return node.ConnectTo(n)
	}

	if _, has := n.Connections[node.Index]; !has {
		n.Connections[node.Index] = node
		node.Connections[n.Index] = n
		return true
	}

	return false
}

func (n *Node) DeleteConnectionTo(node *Node) {
	if node == nil {
		return
	}
	if n.NodeType == CheckNode {
		delete(n.Connections, node.Index)
	} else {
		node.DeleteConnectionTo(n)
	}
}

func (n *Node) OnClick(ctx app.Context, e app.Event) {
	if n.parent == nil {
		panic("No parent")
	}

	if n == n.parent.selectedNode {
		e.StopImmediatePropagation()
		e.PreventDefault()
		return
	}

	if n.parent.selectedNode != nil && n.parent.selectedNode != n {
		n.parent.selectedNode.Selected = false
		n.parent.selectedNode.Highlighted = false

		if n.parent.selectedNode.NodeType != n.NodeType {
			if n.parent.selectedNode.ConnectTo(n) {
				n.parent.addToHistory()
			}
		}

		n.parent.selectedNode.Update()
	}

	n.parent.selectedNode = n
	n.Selected = true
	n.Update()
	e.StopImmediatePropagation()
	e.PreventDefault()
}

func (n *Node) OnMouseEnter(ctx app.Context, e app.Event) {
	n.Highlighted = true
	n.Update()
}
func (n *Node) OnMouseLeave(ctx app.Context, e app.Event) {
	n.Highlighted = false
	n.Update()
}

type svgCanvas struct {
	app.Compo
	keys           map[int]bool
	Scale          int
	history        []string
	nodes          []*Node
	nodeMap        map[NodeType]map[int]*Node
	nodeTypeCounts map[NodeType]int
	posToNode      map[int]map[int]*Node // [X][Y]
	selectedNode   *Node
}

func (sc *svgCanvas) OnMount(ctx app.Context) {
	sc.Scale = 11
	app.Window().AddEventListener("keydown", sc.OnKeyDown)
	app.Window().AddEventListener("keyup", sc.OnKeyUp)
	sc.SetState(`{"Nodes": []}`)
}

func scaleDown(v, scale int) int {
	c := 3 * float64(scale)
	return int(math.Round(float64(v) / c))
}

func scaleUp(v, scale int) int {
	return v * 3 * scale
}

func (sc *svgCanvas) OnClick(ctx app.Context, e app.Event) {
	rect := e.Get("target").Call("getBoundingClientRect")
	ox, oy := e.Get("clientX").Int()-rect.Get("left").Int(), e.Get("clientY").Int()-rect.Get("top").Int()

	x, y := scaleDown(ox, sc.Scale), scaleDown(oy, sc.Scale)

	changed := false
	if sc.spaceFreeAt(x, y) {
		if sc.selectedNode != nil && sc.selectedNode.NodeType == CheckNode {
			sc.createNewNode(VarNode, x, y)
		} else {
			sc.createNewNode(CheckNode, x, y)
		}
		changed = true
	}

	n := sc.posToNode[x][y]

	if n == sc.selectedNode {
		return
	}

	if sc.selectedNode != nil {
		sc.selectedNode.Selected = false
		sc.selectedNode.Update()

		if sc.selectedNode.NodeType != n.NodeType {
			sc.selectedNode.ConnectTo(n)
			sc.Update()
			changed = true
		}

	}
	sc.selectedNode = n
	sc.selectedNode.Selected = true
	if changed {
		sc.addToHistory()
	}
}

func (sc *svgCanvas) spaceFreeAt(x, y int) bool {
	if len(sc.posToNode) == 0 {
		sc.posToNode = make(map[int]map[int]*Node)
	}
	if len(sc.posToNode[x]) == 0 {
		sc.posToNode[x] = make(map[int]*Node)
	}
	ys, has := sc.posToNode[x]
	if !has {
		return true
	}
	_, has = ys[y]
	return !has
}

func (sc *svgCanvas) createNewNode(nodeType NodeType, x, y int) *Node {
	return sc.createNode(nodeType, sc.nodeTypeCounts[nodeType], x, y)
}

func (sc *svgCanvas) createNode(nodeType NodeType, index, x, y int) *Node {
	n := &Node{
		parent:      sc,
		NodeType:    nodeType,
		Index:       index,
		Connections: make(map[int]*Node),
		X:           x,
		Y:           y,
	}

	if len(sc.posToNode[n.X]) == 0 {
		sc.posToNode[n.X] = make(map[int]*Node)
	}
	if _, has := sc.posToNode[n.X][n.Y]; has {
		return nil
	}

	sc.posToNode[n.X][n.Y] = n

	sc.nodes = append(sc.nodes, n)

	sc.nodeMap[n.NodeType][n.Index] = n
	sc.nodeTypeCounts[n.NodeType]++

	return n
}

func (sc *svgCanvas) OnScaleChange(ctx app.Context, e app.Event) {
	sc.Scale, _ = strconv.Atoi(ctx.JSSrc().Get("value").String())

	for _, node := range sc.nodes {
		node.Update()
	}
}

func (sc *svgCanvas) Render() app.UI {
	elems := []any{
		attr.Width(1000),
		attr.Height(1000),
		attr.ViewBox(0, 0, 1000, 1000),
	}

	lines := make([]any, 0)
	for _, a := range sc.nodes {
		if a.NodeType != CheckNode {
			continue
		}
		lines = append(lines, a.RenderLines())
	}
	elems = append(elems, svg.Svg(lines...))
	for _, a := range sc.nodes {
		elems = append(elems, a)
	}

	return svg.Svg(elems...).Style("border", "2px solid black").OnClick(sc.OnClick)
}

func (sc *svgCanvas) addToHistory() {
	str, _ := sc.State()
	sc.history = append(sc.history, string(str))
	fmt.Printf("state:%v", string(str))
}

func (sc *svgCanvas) DeleteNode(n *Node) {
	if len(sc.posToNode[n.X]) == 0 {
		panic("expected to exists")
	}
	if _, has := sc.posToNode[n.X][n.Y]; !has {
		panic("expected to exists")
	}
	delete(sc.posToNode[n.X], n.Y)

	i := slices.IndexFunc(sc.nodes, func(del *Node) bool {
		return del.NodeType == n.NodeType && del.Index == n.Index
	})
	if i == -1 {
		panic("expected to exists")
	}

	sc.nodes[i] = sc.nodes[len(sc.nodes)-1]
	sc.nodes = sc.nodes[:len(sc.nodes)-1]

	delete(sc.nodeMap[n.NodeType], n.Index)
}

func (sc *svgCanvas) OnKeyDown(ctx app.Context, e app.Event) {
	k := e.Get("keyCode").Int()

	_, has := sc.keys[k]
	if !has {
		sc.keys[k] = true
		switch {
		case sc.keys[Esc]:
			sc.selectedNode.Selected = false
			sc.selectedNode.Update()
			sc.selectedNode = nil
			sc.Update()
		case sc.keys[Ctrl] && sc.keys[KeyZ]:
			fmt.Println("Undo")
			if len(sc.history) == 0 {
				e.StopImmediatePropagation()
				return
			}

			sc.history = sc.history[:len(sc.history)-1]
			sc.SetState(sc.history[len(sc.history)-1])
			sc.Update()

		case sc.keys[Del]:
			fmt.Println("Delete")
			node := sc.selectedNode
			sc.selectedNode = nil

			for _, n := range node.Connections {
				node.DeleteConnectionTo(n)
			}
			sc.DeleteNode(node)
			sc.addToHistory()
		}
	}
	e.StopImmediatePropagation()
}

func (sc *svgCanvas) OnKeyUp(ctx app.Context, e app.Event) {
	k := e.Get("keyCode").Int()
	if _, has := sc.keys[k]; has {
		delete(sc.keys, e.Get("keyCode").Int())
		e.StopImmediatePropagation()
	}
}

type nodeJson struct {
	Connections []int
	Index       int
	X, Y        int
	Type        NodeType
}

type svgCanvasJson struct {
	Nodes []nodeJson
}

func (sc *svgCanvas) State() ([]byte, error) {
	checkDelta, varDelta := math.MaxInt, math.MaxInt

	for _, n := range sc.nodes {
		if n.NodeType == CheckNode {
			if n.Index < checkDelta {
				checkDelta = n.Index
			}
		} else {
			if n.Index < varDelta {
				varDelta = n.Index
			}
		}
	}

	nodes := make([]nodeJson, 0)
	for _, n := range sc.nodes {
		connections := make([]int, 0)

		for c := range n.Connections {
			connections = append(connections, c)
		}
		delta := varDelta
		if n.NodeType == CheckNode {
			delta = checkDelta
		}

		nodes = append(nodes, nodeJson{
			Index:       n.Index - delta,
			Type:        n.NodeType,
			X:           n.X,
			Y:           n.Y,
			Connections: connections,
		})
	}

	return json.MarshalIndent(svgCanvasJson{Nodes: nodes}, " ", " ")
}

func (sc *svgCanvas) SetState(state string) {
	sc.selectedNode = nil
	sc.keys = make(map[int]bool)
	sc.nodes = make([]*Node, 0)
	sc.nodeMap = map[NodeType]map[int]*Node{CheckNode: {}, VarNode: {}}
	sc.posToNode = make(map[int]map[int]*Node)
	sc.nodeTypeCounts = map[NodeType]int{CheckNode: 0, VarNode: 0}
	sc.Update()

	var tmp svgCanvasJson
	err := json.Unmarshal([]byte(state), &tmp)
	if err != nil {
		panic(err)
	}

	indexToVars := make(map[int]*Node)
	indexToChecks := make(map[int]*Node)
	for _, node := range tmp.Nodes {
		n := sc.createNode(node.Type, node.Index, node.X, node.Y)
		if node.Type == VarNode {
			indexToVars[node.Index] = n
		} else {
			indexToChecks[node.Index] = n
		}
	}

	for _, node := range tmp.Nodes {
		if node.Type == VarNode {
			continue
		}

		n := indexToChecks[node.Index]

		for _, c := range node.Connections {
			n.ConnectTo(indexToVars[c])
		}
	}

	sc.Update()
}

func main() {
	// Components routing:
	app.Route("/", &svgCanvas{})
	app.RunWhenOnBrowser()

	// HTTP routing:
	http.Handle("/", &app.Handler{
		Name:        "Hello",
		Description: "An Hello World! example",
	})

	port := 8008
	fmt.Printf("http://localhost:%v\n", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%v", port), nil); err != nil {
		log.Fatal(err)
	}
}
