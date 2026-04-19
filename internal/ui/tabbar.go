package ui

import (
	"image"
	"strings"
	"time"
	"tracto/internal/utils"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func measureTabWidth(gtx layout.Context, th *material.Theme, cleanTitle string) int {
	words := strings.Fields(cleanTitle)

	var maxW int
	if len(words) <= 1 {
		if len(words) == 0 {
			cleanTitle = "New request"
		}
		maxW = measureTextWidth(gtx, th, unit.Sp(12), font.Font{}, cleanTitle)
	} else {
		mid := (len(words) + 1) / 2
		line1 := strings.Join(words[:mid], " ")
		line2 := strings.Join(words[mid:], " ")
		w1 := measureTextWidth(gtx, th, unit.Sp(12), font.Font{}, line1)
		w2 := measureTextWidth(gtx, th, unit.Sp(12), font.Font{}, line2)
		if w1 > w2 {
			maxW = w1
		} else {
			maxW = w2
		}
	}

	totalW := maxW + gtx.Dp(unit.Dp(52))
	maxWidthLimit := gtx.Dp(unit.Dp(200))
	if totalW > maxWidthLimit {
		return maxWidthLimit
	}
	return totalW
}

func (ui *AppUI) closeTab(idx int) {
	if idx < 0 || idx >= len(ui.Tabs) {
		return
	}
	tab := ui.Tabs[idx]
	tab.cancelRequest()
	tab.cleanupRespFile()
	delete(ui.tabWidthCache, tab)
	ui.Tabs = append(ui.Tabs[:idx], ui.Tabs[idx+1:]...)
	if ui.ActiveIdx >= idx && ui.ActiveIdx > 0 {
		ui.ActiveIdx--
	} else if ui.ActiveIdx >= len(ui.Tabs) {
		ui.ActiveIdx = len(ui.Tabs) - 1
	}
	ui.saveState()
}

func (ui *AppUI) layoutTabBar(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		tabHeight := gtx.Dp(unit.Dp(36))
		closeBtnWidth := gtx.Dp(unit.Dp(28))
		addBtnW := gtx.Dp(unit.Dp(36))
		maxWidth := gtx.Constraints.Max.X - 2
		if maxWidth < 0 {
			maxWidth = 0
		}

		tabs := ui.tabInfoBuf[:0]
		for i, tab := range ui.Tabs {
			cache, ok := ui.tabWidthCache[tab]
			if !ok || cache.title != tab.Title || cache.ppdp != gtx.Metric.PxPerDp {
				natW := measureTabWidth(gtx, ui.Theme, tab.getCleanTitle())
				ui.tabWidthCache[tab] = cachedTab{title: tab.Title, width: natW, ppdp: gtx.Metric.PxPerDp}
				cache = ui.tabWidthCache[tab]
			}
			tabs = append(tabs, tabBarInfo{Idx: i, NatWidth: cache.width})
		}
		ui.tabInfoBuf = tabs

		rows := ui.tabRowsBuf[:0]
		var currentX int
		currentRow := ui.tabRowBuf[:0]

		for i, t := range tabs {
			w := t.NatWidth
			if currentX > 0 && currentX+w > maxWidth {
				rows = append(rows, currentRow)
				currentRow = nil
				currentX = 0
			}
			currentRow = append(currentRow, i)
			currentX += w
		}

		if currentX > 0 && currentX+addBtnW > maxWidth {
			rows = append(rows, currentRow)
			currentRow = nil
			currentX = 0
		}
		currentRow = append(currentRow, -1)
		rows = append(rows, currentRow)
		ui.tabRowsBuf = rows
		ui.tabRowBuf = currentRow

		for rIdx, row := range rows {
			isLastRow := rIdx == len(rows)-1

			rowTabsNatW := 0
			rowHasAddBtn := false
			for _, i := range row {
				if i >= 0 {
					rowTabsNatW += tabs[i].NatWidth
				} else {
					rowHasAddBtn = true
				}
			}

			rowTotalNatW := rowTabsNatW
			if rowHasAddBtn {
				rowTotalNatW += addBtnW
			}

			if isLastRow {
				for _, i := range row {
					if i >= 0 {
						tabs[i].FinalWidth = tabs[i].NatWidth
					}
				}
				continue
			}

			extraSpace := maxWidth - rowTotalNatW
			if extraSpace > 0 && rowTabsNatW > 0 {
				allocated := 0
				lastTabInRowIdx := -1
				for j, i := range row {
					if i >= 0 {
						lastTabInRowIdx = j
					}
				}

				for j, i := range row {
					if i >= 0 {
						var add int
						if j == lastTabInRowIdx {
							add = extraSpace - allocated
						} else {
							share := float32(tabs[i].NatWidth) / float32(rowTabsNatW)
							add = int(float32(extraSpace) * share)
						}
						allocated += add
						tabs[i].FinalWidth = tabs[i].NatWidth + add
					}
				}
			} else {
				for _, i := range row {
					if i >= 0 {
						tabs[i].FinalWidth = tabs[i].NatWidth
					}
				}
			}
		}

		th := float32(tabHeight)

		tabIdxAtXY := func(x, y float32) int {
			rowIdx := int(y / th)
			if rowIdx < 0 {
				rowIdx = 0
			}
			if rowIdx >= len(rows) {
				rowIdx = len(rows) - 1
			}
			row := rows[rowIdx]
			acc := float32(0)
			for _, tIdx := range row {
				var w float32
				if tIdx < 0 {
					w = float32(addBtnW)
				} else {
					w = float32(tabs[tIdx].FinalWidth)
				}
				if x < acc+w {
					return tIdx
				}
				acc += w
			}
			if len(row) > 0 {
				last := row[len(row)-1]
				if last == -1 && len(row) > 1 {
					last = row[len(row)-2]
				}
				if last >= 0 {
					return last
				}
			}
			return -1
		}

		tabPosInRow := func(idx int) (row int, xOff float32) {
			for r, rr := range rows {
				x := float32(0)
				for _, tIdx := range rr {
					if tIdx == idx {
						return r, x
					}
					if tIdx >= 0 {
						x += float32(tabs[tIdx].FinalWidth)
					}
				}
			}
			return 0, 0
		}

		swapTabs := func(a, b int) {
			ui.Tabs[a], ui.Tabs[b] = ui.Tabs[b], ui.Tabs[a]
			tabs[a], tabs[b] = tabs[b], tabs[a]
			if ui.ActiveIdx == a {
				ui.ActiveIdx = b
			} else if ui.ActiveIdx == b {
				ui.ActiveIdx = a
			}
		}

		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: &ui.TabDragTag,
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
			})
			if !ok {
				break
			}
			if pe, ok := ev.(pointer.Event); ok {
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						hit := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if hit >= 0 {
							hitRow, xOff := tabPosInRow(hit)
							ui.TabDragIdx = hit
							ui.TabDragOriginX = pe.Position.X - xOff
							ui.TabDragOriginY = pe.Position.Y - float32(hitRow)*th
							ui.TabDragCurrentX = pe.Position.X
							ui.TabDragCurrentY = pe.Position.Y
							ui.TabDragging = false
							ui.TabDragPressTime = gtx.Now
						}
					} else if pe.Buttons.Contain(pointer.ButtonSecondary) {
						hit := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if hit >= 0 {
							ui.TabCtxMenuOpen = true
							ui.TabCtxMenuIdx = hit
							ui.TabCtxMenuPos = pe.Position
						}
					}
				case pointer.Drag:
					ui.TabDragCurrentX = pe.Position.X
					ui.TabDragCurrentY = pe.Position.Y
					if !ui.TabDragging && ui.TabDragIdx >= 0 {
						elapsed := gtx.Now.Sub(ui.TabDragPressTime)
						dx := pe.Position.X - (ui.TabDragOriginX + float32(tabs[ui.TabDragIdx].FinalWidth)/2)
						dy := pe.Position.Y - (ui.TabDragOriginY + th/2)
						dist := dx*dx + dy*dy
						if elapsed > 150*time.Millisecond && dist > 100 {
							ui.TabDragging = true
						}
					}
					if ui.TabDragging && ui.TabDragIdx >= 0 && ui.TabDragIdx < len(ui.Tabs) {
						target := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if target >= 0 && target != ui.TabDragIdx {
							old := ui.TabDragIdx
							if target > old {
								for i := old; i < target; i++ {
									swapTabs(i, i+1)
								}
							} else {
								for i := old; i > target; i-- {
									swapTabs(i, i-1)
								}
							}
							ui.TabDragIdx = target
						}
					}
				case pointer.Release, pointer.Cancel:
					if ui.TabDragging {
						ui.saveState()
					}
					ui.TabDragging = false
					ui.TabDragIdx = -1
				}
			}
		}

		tabBarHeight := len(rows) * tabHeight
		clipStack := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, tabBarHeight)}.Push(gtx.Ops)
		passStack := pointer.PassOp{}.Push(gtx.Ops)
		event.Op(gtx.Ops, &ui.TabDragTag)

		var dragTabOX, dragTabOY int
		var dragTabW int

		rowChildren := make([]layout.FlexChild, len(rows))
		for i := range rows {
			rIdx := i
			row := rows[rIdx]
			rowChildren[rIdx] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(row))

				for j, tIdx := range row {
					if tIdx >= 0 {
						idx := tIdx
						finalW := tabs[idx].FinalWidth
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = finalW
							gtx.Constraints.Max.X = finalW
							gtx.Constraints.Min.Y = tabHeight
							gtx.Constraints.Max.Y = tabHeight

							if ui.TabDragging && ui.TabDragIdx == idx {
								dragTabOX = int(ui.TabDragCurrentX - ui.TabDragOriginX)
								dragTabOY = int(ui.TabDragCurrentY - ui.TabDragOriginY)
								dragTabW = finalW
								paint.FillShape(gtx.Ops, colorBgDragHolder, clip.Rect{Max: image.Pt(finalW, tabHeight)}.Op())
								return layout.Dimensions{Size: image.Pt(finalW, tabHeight)}
							}

							tab := ui.Tabs[idx]
							if tab.TabBtn.Clicked(gtx) {
								if !ui.TabDragging {
									ui.ActiveIdx = idx
									ui.TabCtxMenuOpen = false
									ui.revealLinkedNode(tab)
								}
							}

							bgColor := colorBgDark
							fgColor := colorFgMuted
							if idx == ui.ActiveIdx {
								bgColor = colorBg
								fgColor = colorWhite
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
									if idx == ui.ActiveIdx {
										paint.FillShape(gtx.Ops, colorAccent, clip.Rect{Max: image.Point{X: gtx.Constraints.Min.X, Y: gtx.Dp(unit.Dp(2))}}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
											gtx.Constraints.Min.X = gtx.Constraints.Max.X
											return material.Clickable(gtx, &tab.TabBtn, func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min = gtx.Constraints.Max
												return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(10), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														cleanTitle := tab.getCleanTitle()
														if tab.IsDirty {
															cleanTitle = "● " + cleanTitle
														}
														lbl := material.Label(ui.Theme, unit.Sp(12), cleanTitle)
														lbl.Color = fgColor
														lbl.MaxLines = 2
														lbl.Truncator = "..."
														return lbl.Layout(gtx)
													})
												})
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											gtx.Constraints.Min.X = closeBtnWidth
											gtx.Constraints.Max.X = closeBtnWidth
											gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
											return material.Clickable(gtx, &tab.CloseBtn, func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min = gtx.Constraints.Max
												return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													size := gtx.Dp(unit.Dp(16))
													gtx.Constraints.Min = image.Point{X: size, Y: size}
													gtx.Constraints.Max = gtx.Constraints.Min
													return iconClose.Layout(gtx, fgColor)
												})
											})
										}),
									)
								}),
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									maxX := gtx.Constraints.Min.X
									maxY := gtx.Constraints.Min.Y
									t := 1
									if gtx.Dp(1) > 1 {
										t = gtx.Dp(1)
									}
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
									if rIdx == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
									}
									if j == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
							)
						}))
					} else {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(unit.Dp(36))
							gtx.Constraints.Max.X = gtx.Constraints.Min.X
							gtx.Constraints.Min.Y = tabHeight
							gtx.Constraints.Max.Y = tabHeight

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, colorBgDark, clip.Rect{Max: gtx.Constraints.Min}.Op())
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(ui.Theme, &ui.AddTabBtn, "+")
									btn.Background = colorBgDark
									btn.Color = ui.Theme.Palette.Fg
									btn.TextSize = unit.Sp(16)
									btn.CornerRadius = unit.Dp(0)
									btn.Inset = layout.Inset{}
									gtx.Constraints.Min = gtx.Constraints.Max
									return btn.Layout(gtx)
								}),
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									maxX := gtx.Constraints.Min.X
									maxY := gtx.Constraints.Min.Y
									t := 1
									if gtx.Dp(1) > 1 {
										t = gtx.Dp(1)
									}
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
									paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
									if rIdx == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
									}
									if j == 0 {
										paint.FillShape(gtx.Ops, colorBorder, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
							)
						}))
					}
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			})
		}
		listDims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)

		passStack.Pop()
		clipStack.Pop()

		if ui.TabDragging && ui.TabDragIdx >= 0 && ui.TabDragIdx < len(ui.Tabs) {
			dragMacro := op.Record(gtx.Ops)
			op.Offset(image.Pt(dragTabOX, dragTabOY)).Add(gtx.Ops)
			dIdx := ui.TabDragIdx
			dTab := ui.Tabs[dIdx]
			dW := dragTabW
			if dW <= 0 {
				dW = tabs[dIdx].FinalWidth
			}
			dGtx := gtx
			dGtx.Constraints.Min = image.Pt(dW, tabHeight)
			dGtx.Constraints.Max = dGtx.Constraints.Min
			paint.FillShape(dGtx.Ops, colorBgDragGhost, clip.Rect{Max: dGtx.Constraints.Min}.Op())
			paint.FillShape(dGtx.Ops, colorAccent, clip.Rect{Max: image.Point{X: dW, Y: dGtx.Dp(unit.Dp(2))}}.Op())
			layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(10), Right: unit.Dp(6)}.Layout(dGtx, func(gtx layout.Context) layout.Dimensions {
				t := utils.SanitizeText(dTab.Title)
				t = strings.ReplaceAll(t, "\n", " ")
				if strings.TrimSpace(t) == "" {
					t = "New request"
				}
				if dTab.IsDirty {
					t = "● " + t
				}
				lbl := material.Label(ui.Theme, unit.Sp(12), t)
				lbl.Color = colorWhite
				lbl.MaxLines = 2
				lbl.Truncator = "..."
				return lbl.Layout(gtx)
			})
			op.Defer(gtx.Ops, dragMacro.Stop())
		}

		return listDims
	})
}
