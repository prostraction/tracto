package ui

import (
	"bytes"
	"image"
	"io"
	"os"
	"path/filepath"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/transfer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, colorBgDark, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size

	bgClip := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, transfer.TargetFilter{Target: &ui.SidebarDropTag, Type: "text/plain"})
	event.Op(gtx.Ops, transfer.TargetFilter{Target: &ui.SidebarDropTag, Type: "application/json"})
	event.Op(gtx.Ops, &ui.ColList)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.ColList,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok && ui.RenamingNode != nil {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	bgClip.Pop()

	var moved bool
	var finalY float32
	var released bool

	for {
		e, ok := ui.SidebarEnvDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			ui.SidebarEnvDragY = e.Position.Y
		case pointer.Drag:
			finalY = e.Position.Y
			moved = true
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	if moved {
		delta := finalY - ui.SidebarEnvDragY
		oldHeight := ui.SidebarEnvHeight
		ui.SidebarEnvHeight -= int(delta)
		minEnvHeight := gtx.Dp(unit.Dp(110))
		maxEnvHeight := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(100))
		if ui.SidebarEnvHeight < minEnvHeight {
			ui.SidebarEnvHeight = minEnvHeight
		}
		if ui.SidebarEnvHeight > maxEnvHeight && maxEnvHeight > minEnvHeight {
			ui.SidebarEnvHeight = maxEnvHeight
		}
		actualDelta := oldHeight - ui.SidebarEnvHeight
		ui.SidebarEnvDragY = finalY - float32(actualDelta)
		ui.Window.Invalidate()
	}
	if released {
		ui.saveState()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.ColsHeaderClick.Clicked(gtx) {
						ui.ColsExpanded = !ui.ColsExpanded
					}
					for ui.ImportBtn.Clicked(gtx) {
						go func() {
							file, err := ui.Explorer.ChooseFile("json")
							if err == nil && file != nil {
								data, err := io.ReadAll(file)
								file.Close()
								if err == nil {
									id, _ := saveCollectionRaw(data)
									col, err := ParseCollection(bytes.NewReader(data), id)
									if err == nil && col != nil {
										ui.ColLoadedChan <- &CollectionUI{Data: col}
										ui.Window.Invalidate()
									}
								}
							}
						}()
					}

					return material.Clickable(gtx, &ui.ColsHeaderClick, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									txt := "►"
									if ui.ColsExpanded {
										txt = "▼"
									}
									lbl := material.Label(ui.Theme, unit.Sp(10), txt)
									lbl.Color = colorFgMuted
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.Theme, unit.Sp(12), "Collections")
									lbl.Font.Weight = font.Bold
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.ImportBtn, "Import")
									btn.Background = colorAccent
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(10)
									btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}
									return btn.Layout(gtx)
								}),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
					paint.FillShape(gtx.Ops, colorBorder, rect.Op())
					return layout.Dimensions{Size: rect.Max}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !ui.ColsExpanded {
						return layout.Dimensions{}
					}
					if len(ui.Collections) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(12), "No collections loaded")
							lbl.Color = colorFgMuted
							lbl.Alignment = text.Middle
							return lbl.Layout(gtx)
						})
					}

					commitRename := func(n *CollectionNode) {
						if n == nil || !n.IsRenaming {
							return
						}
						n.Name = n.NameEditor.Text()
						if n.Request != nil {
							n.Request.Name = n.Name
						}
						n.IsRenaming = false
						n.RenamingFocused = false
						if ui.RenamingNode == n {
							ui.RenamingNode = nil
						}
						ui.markCollectionDirty(n.Collection)
					}

					var updateCols bool
					dim := material.List(ui.Theme, &ui.ColList).Layout(gtx, len(ui.VisibleCols), func(gtx layout.Context, i int) layout.Dimensions {
						node := ui.VisibleCols[i]

						if node.IsRenaming {
							for {
								ev, ok := node.NameEditor.Update(gtx)
								if !ok {
									break
								}
								if _, ok := ev.(widget.SubmitEvent); ok {
									commitRename(node)
								}
							}

							for {
								ev, ok := gtx.Event(
									key.Filter{Focus: &node.NameEditor, Name: "S", Required: key.ModShortcut},
									key.Filter{Focus: &node.NameEditor, Name: key.NameEscape},
								)
								if !ok {
									break
								}
								if e, ok := ev.(key.Event); ok && e.State == key.Press {
									if e.Name == key.NameEscape {
										node.IsRenaming = false
										node.RenamingFocused = false
										if ui.RenamingNode == node {
											ui.RenamingNode = nil
										}
									} else {
										commitRename(node)
									}
								}
							}
						}

						if node.IsRenaming {
							ui.RenamingNode = node
							if gtx.Focused(&node.NameEditor) {
								node.RenamingFocused = true
							} else if node.RenamingFocused {
								commitRename(node)
							} else {
								gtx.Execute(key.FocusCmd{Tag: &node.NameEditor})
							}
						}

						for node.MenuBtn.Clicked(gtx) {
							if ui.RenamingNode != nil && ui.RenamingNode != node {
								commitRename(ui.RenamingNode)
							}
							if !node.MenuOpen {
								for _, n := range ui.VisibleCols {
									n.MenuOpen = false
								}
							}
							node.MenuOpen = !node.MenuOpen
							updateCols = true
						}

						if node.MenuOpen {
							for node.AddReqBtn.Clicked(gtx) {
								commitRename(ui.RenamingNode)
								newNode := &CollectionNode{
									Name:       "New Request",
									Request:    &ParsedRequest{Method: "GET"},
									Depth:      node.Depth + 1,
									Parent:     node,
									Collection: node.Collection,
									IsRenaming: true,
								}
								newNode.NameEditor.SetText("New Request")
								node.Children = append(node.Children, newNode)
								node.Expanded = true
								node.MenuOpen = false
								updateCols = true
								ui.markCollectionDirty(node.Collection)
							}

							for node.AddFldBtn.Clicked(gtx) {
								commitRename(ui.RenamingNode)
								newNode := &CollectionNode{
									Name:       "New Folder",
									IsFolder:   true,
									Depth:      node.Depth + 1,
									Parent:     node,
									Collection: node.Collection,
									IsRenaming: true,
								}
								newNode.NameEditor.SetText("New Folder")
								node.Children = append(node.Children, newNode)
								node.Expanded = true
								node.MenuOpen = false
								updateCols = true
								ui.markCollectionDirty(node.Collection)
							}

							for node.EditBtn.Clicked(gtx) {
								commitRename(ui.RenamingNode)
								node.IsRenaming = true
								node.NameEditor.SetText(node.Name)
								node.MenuOpen = false
								ui.RenamingNode = node
							}

							for node.DupBtn.Clicked(gtx) {
								dup := cloneNode(node, node.Parent)
								if node.Parent != nil {
									node.Parent.Children = append(node.Parent.Children, dup)
								}
								node.MenuOpen = false
								updateCols = true
								ui.markCollectionDirty(node.Collection)
							}

							for node.DelBtn.Clicked(gtx) {
								if node.Parent != nil {
									for idx, c := range node.Parent.Children {
										if c == node {
											node.Parent.Children = append(node.Parent.Children[:idx], node.Parent.Children[idx+1:]...)
											break
										}
									}
									ui.markCollectionDirty(node.Collection)
								} else {
									for idx, c := range ui.Collections {
										if c.Data == node.Collection {
											ui.Collections = append(ui.Collections[:idx], ui.Collections[idx+1:]...)
											break
										}
									}
									os.Remove(filepath.Join(getCollectionsDir(), node.Collection.ID+".json"))
								}
								for _, tab := range ui.Tabs {
									if tab.LinkedNode == node {
										tab.LinkedNode = nil
									}
								}
								node.MenuOpen = false
								updateCols = true
							}
						}

						for node.Click.Clicked(gtx) {
							if ui.RenamingNode != nil && ui.RenamingNode != node {
								commitRename(ui.RenamingNode)
							}
							if node.IsRenaming {
								continue
							}
							if node.IsFolder {
								node.Expanded = !node.Expanded
								updateCols = true
							} else if node.Request != nil {
								ui.openRequestInTab(node)
							}
						}

						return layout.Inset{
							Top: unit.Dp(1), Bottom: unit.Dp(1),
							Left: unit.Dp(8), Right: unit.Dp(8),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							isActiveNode := false
							if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
								isActiveNode = ui.Tabs[ui.ActiveIdx].LinkedNode == node
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									if isActiveNode {
										paint.FillShape(gtx.Ops, colorAccentDim, clip.Rect{Max: gtx.Constraints.Min}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											return material.Clickable(gtx, &node.Click, func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min.X = gtx.Constraints.Max.X
												return layout.Inset{
													Top: unit.Dp(4), Bottom: unit.Dp(4),
													Left:  unit.Dp(float32(node.Depth * 12)),
													Right: unit.Dp(4),
												}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													children := make([]layout.FlexChild, 0, 3)
													if node.IsFolder {
														txt := node.Name
														if node.Expanded {
															txt = "▼ " + txt
														} else {
															txt = "► " + txt
														}
														children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															if node.IsRenaming {
																return TextField(gtx, ui.Theme, &node.NameEditor, "", false, nil, 0, unit.Sp(12))
															}
															lbl := material.Label(ui.Theme, unit.Sp(12), txt)
															lbl.Alignment = text.Start
															if node.Depth == 0 {
																lbl.Font.Weight = font.Bold
															}
															return layout.W.Layout(gtx, lbl.Layout)
														}))
													} else if node.Request != nil {
														children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															lbl := material.Label(ui.Theme, unit.Sp(10), node.Request.Method)
															lbl.Color = getMethodColor(node.Request.Method)
															return lbl.Layout(gtx)
														}))
														children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
														children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															if node.IsRenaming {
																return TextField(gtx, ui.Theme, &node.NameEditor, "", false, nil, 0, unit.Sp(12))
															}
															lbl := material.Label(ui.Theme, unit.Sp(12), node.Name)
															lbl.Alignment = text.Start
															return layout.W.Layout(gtx, lbl.Layout)
														}))
													}
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
												})
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											btn := material.Button(ui.Theme, &node.MenuBtn, "⋮")
											btn.Background = colorTransparent
											btn.Color = ui.Theme.Palette.Fg
											btn.Inset = layout.UniformInset(unit.Dp(2))
											btn.TextSize = unit.Sp(14)
											dims := btn.Layout(gtx)
											node.MenuBtnWidth = dims.Size.X
											return dims
										}),
									)
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									if !node.MenuOpen {
										return layout.Dimensions{}
									}

									macro := op.Record(gtx.Ops)
									menuWidth := gtx.Dp(unit.Dp(166))
									menuX := gtx.Constraints.Max.X - menuWidth
									if menuX < 0 {
										menuX = 0
									}
									op.Offset(image.Pt(menuX, gtx.Dp(unit.Dp(24)))).Add(gtx.Ops)
									widget.Border{
										Color:        colorBorderLight,
										CornerRadius: unit.Dp(4),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												paint.FillShape(gtx.Ops, colorBgPopup, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
												defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
												event.Op(gtx.Ops, &node.MenuOpen)
												for {
													_, ok := gtx.Event(pointer.Filter{Target: &node.MenuOpen, Kinds: pointer.Press})
													if !ok {
														break
													}
												}
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													actions := make([]layout.FlexChild, 0, 5)
													if node.IsFolder || node.Depth == 0 {
														actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return menuOption(gtx, ui.Theme, &node.AddReqBtn, "Add Request", iconAddReq)
														}))
														actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return menuOption(gtx, ui.Theme, &node.AddFldBtn, "Add Folder", iconAddFld)
														}))
													}
													actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return menuOption(gtx, ui.Theme, &node.EditBtn, "Rename", iconRename)
													}))
													if node.Depth > 0 {
														actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return menuOption(gtx, ui.Theme, &node.DupBtn, "Duplicate", iconDup)
														}))
													}
													actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return menuOption(gtx, ui.Theme, &node.DelBtn, "Delete", iconDel)
													}))
													return layout.Flex{Axis: layout.Vertical}.Layout(gtx, actions...)
												})
											}),
										)
									})
									call := macro.Stop()
									op.Defer(gtx.Ops, call)

									return layout.Dimensions{}
								}),
							)
						})
					})

					if updateCols {
						ui.updateVisibleCols()
					}

					return dim
				}),
			)
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(6))}
			rect := clip.Rect{Max: size}
			defer rect.Push(gtx.Ops).Pop()
			pointer.CursorRowResize.Add(gtx.Ops)
			ui.SidebarEnvDrag.Add(gtx.Ops)
			paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Max: size}.Op())
			return layout.Dimensions{Size: size}
		}),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = ui.SidebarEnvHeight
			gtx.Constraints.Max.Y = ui.SidebarEnvHeight

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ui.EnvsHeaderClick.Clicked(gtx) {
						ui.EnvsExpanded = !ui.EnvsExpanded
					}
					for ui.ImportEnvBtn.Clicked(gtx) {
						go func() {
							file, err := ui.Explorer.ChooseFile("json")
							if err == nil && file != nil {
								data, err := io.ReadAll(file)
								file.Close()
								if err == nil {
									id, _ := saveEnvironmentRaw(data)
									env, err := ParseEnvironment(bytes.NewReader(data), id)
									if err == nil && env != nil {
										ui.EnvLoadedChan <- &EnvironmentUI{Data: env}
										ui.Window.Invalidate()
									}
								}
							}
						}()
					}

					return material.Clickable(gtx, &ui.EnvsHeaderClick, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									txt := "►"
									if ui.EnvsExpanded {
										txt = "▼"
									}
									lbl := material.Label(ui.Theme, unit.Sp(10), txt)
									lbl.Color = colorFgMuted
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.Theme, unit.Sp(12), "Environments")
									lbl.Font.Weight = font.Bold
									return lbl.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.ImportEnvBtn, "Import")
									btn.Background = colorBgField
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(10)
									btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}
									return btn.Layout(gtx)
								}),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
					paint.FillShape(gtx.Ops, colorBorder, rect.Op())
					return layout.Dimensions{Size: rect.Max}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !ui.EnvsExpanded {
						return layout.Dimensions{}
					}
					if len(ui.Environments) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(12), "No environments loaded")
							lbl.Color = colorFgMuted
							lbl.Alignment = text.Middle
							return lbl.Layout(gtx)
						})
					}

					return material.List(ui.Theme, &ui.EnvList).Layout(gtx, len(ui.Environments), func(gtx layout.Context, idx int) layout.Dimensions {
						env := ui.Environments[idx]
						return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(0), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							isActive := ui.ActiveEnvID == env.Data.ID

							var clicked bool
							for env.SelectBtn.Clicked(gtx) {
								clicked = true
							}
							for env.Click.Clicked(gtx) {
								clicked = true
							}
							if clicked {
								if isActive {
									ui.ActiveEnvID = ""
								} else {
									ui.ActiveEnvID = env.Data.ID
								}
								ui.activeEnvDirty = true
								ui.saveState()
								ui.Window.Invalidate()
							}

							for env.EditBtn.Clicked(gtx) {
								ui.EditingEnv = env
								env.initEditor()
								ui.Window.Invalidate()
							}

							bgColor := colorBgDark
							if isActive {
								bgColor = colorBg
							}
							if env.Click.Hovered() {
								bgColor = colorBgHover
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									return material.Clickable(gtx, &env.Click, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
										rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
										paint.FillShape(gtx.Ops, bgColor, rect.Op(gtx.Ops))
										if isActive {
											paint.FillShape(gtx.Ops, ui.Theme.Palette.ContrastBg, clip.Rect{Max: image.Point{X: gtx.Dp(unit.Dp(2)), Y: gtx.Constraints.Min.Y}}.Op())
										}
										return layout.Dimensions{Size: gtx.Constraints.Min}
									})
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Constraints.Max.X
									return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												lbl := material.Label(ui.Theme, unit.Sp(12), env.Data.Name)
												lbl.MaxLines = 1
												lbl.Truncator = "..."
												if isActive {
													lbl.Font.Weight = font.Bold
												}
												return lbl.Layout(gtx)
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return material.Clickable(gtx, &env.SelectBtn, func(gtx layout.Context) layout.Dimensions {
													size := gtx.Dp(18)
													gtx.Constraints.Min = image.Pt(size, size)
													gtx.Constraints.Max = gtx.Constraints.Min
													iconCol := colorFgMuted
													if isActive {
														iconCol = ui.Theme.Palette.ContrastBg
													} else if env.SelectBtn.Hovered() {
														iconCol = ui.Theme.Palette.Fg
													}
													return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
														return iconCheck.Layout(gtx, iconCol)
													})
												})
											}),
											layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return material.Clickable(gtx, &env.EditBtn, func(gtx layout.Context) layout.Dimensions {
													size := gtx.Dp(18)
													gtx.Constraints.Min = image.Pt(size, size)
													gtx.Constraints.Max = gtx.Constraints.Min
													iconCol := colorFgMuted
													if env.EditBtn.Hovered() {
														iconCol = ui.Theme.Palette.Fg
													}
													return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
														return iconSettings.Layout(gtx, iconCol)
													})
												})
											}),
										)
									})
								}),
							)
						})
					})
				}),
			)
		}),
	)
}
