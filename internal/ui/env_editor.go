package ui

import (
	"image"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func (ui *AppUI) commitEditingEnv() {
	env := ui.EditingEnv
	if env == nil || env.Data == nil {
		return
	}
	env.Data.Name = env.NameEditor.Text()
	env.Data.Vars = nil
	for _, r := range env.Rows {
		k := r.KeyEditor.Text()
		if k == "" {
			continue
		}
		env.Data.Vars = append(env.Data.Vars, EnvVar{
			Key:     k,
			Value:   r.ValEditor.Text(),
			Enabled: r.Enabled.Value,
		})
	}
	SaveEnvironment(env.Data)
	ui.activeEnvDirty = true
}

func (ui *AppUI) layoutEnvEditor(gtx layout.Context) layout.Dimensions {
	env := ui.EditingEnv

	for env.BackBtn.Clicked(gtx) {
		ui.commitEditingEnv()
		ui.EditingEnv = nil
		ui.Window.Invalidate()
		return layout.Dimensions{}
	}
	for env.AddBtn.Clicked(gtx) {
		r := &EnvVarRow{}
		r.Enabled.Value = true
		env.Rows = append(env.Rows, r)
		ui.Window.Invalidate()
	}
	for env.SaveBtn.Clicked(gtx) {
		ui.commitEditingEnv()
		ui.Window.Invalidate()
	}
	for i := 0; i < len(env.Rows); i++ {
		if env.Rows[i].DelBtn.Clicked(gtx) {
			env.Rows = append(env.Rows[:i], env.Rows[i+1:]...)
			i--
			ui.Window.Invalidate()
		}
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &env.BackBtn, func(gtx layout.Context) layout.Dimensions {
							bg := colorBorder
							if env.BackBtn.Hovered() {
								bg = colorBorderLight
							}
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
							return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
										return iconBack.Layout(gtx, ui.Theme.Palette.Fg)
									}),
								)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return TextField(gtx, ui.Theme, &env.NameEditor, "Environment Name", true, nil, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &env.SaveBtn, func(gtx layout.Context) layout.Dimensions {
							size := gtx.Dp(28)
							gtx.Constraints.Min = image.Pt(size, size)
							gtx.Constraints.Max = gtx.Constraints.Min
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							bg := ui.Theme.Palette.ContrastBg
							if env.SaveBtn.Hovered() {
								bg = colorAccentHover
							}
							paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min = image.Pt(gtx.Dp(18), gtx.Dp(18))
								return iconSave.Layout(gtx, ui.Theme.Palette.ContrastFg)
							})
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(30)
						return layout.Dimensions{Size: image.Pt(gtx.Dp(30), 0)}
					}),
					layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(12), "Key")
						lbl.Font.Weight = font.Bold
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(12), "Value")
						lbl.Font.Weight = font.Bold
						lbl.Color = colorFgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(28)
						return layout.Dimensions{Size: image.Pt(gtx.Dp(28), 0)}
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(ui.Theme, &env.List).Layout(gtx, len(env.Rows)+1, func(gtx layout.Context, i int) layout.Dimensions {
					if i == len(env.Rows) {
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(ui.Theme, &env.AddBtn, "+ Add Variable")
							btn.Background = colorBorder
							btn.Color = ui.Theme.Palette.Fg
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.UniformInset(unit.Dp(8))
							return btn.Layout(gtx)
						})
					}

					r := env.Rows[i]
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Dp(30)
								return material.CheckBox(ui.Theme, &r.Enabled, "").Layout(gtx)
							}),
							layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
								return TextField(gtx, ui.Theme, &r.KeyEditor, "Key", true, nil, 0, unit.Sp(12))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Flexed(0.45, func(gtx layout.Context) layout.Dimensions {
								return TextField(gtx, ui.Theme, &r.ValEditor, "Value", true, nil, 0, unit.Sp(12))
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								size := gtx.Dp(28)
								gtx.Constraints.Min = image.Pt(size, size)
								gtx.Constraints.Max = gtx.Constraints.Min
								return material.Clickable(gtx, &r.DelBtn, func(gtx layout.Context) layout.Dimensions {
									rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
									bg := colorBorder
									iconColor := ui.Theme.Palette.Fg
									if r.DelBtn.Hovered() {
										bg = colorDanger
										iconColor = colorDangerFg
									}
									paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
										return iconClose.Layout(gtx, iconColor)
									})
								})
							}),
						)
					})
				})
			}),
		)
	})
}

func (ui *AppUI) saveVarPopup() {
	if ui.VarPopupEnvID != "" {
		for _, env := range ui.Environments {
			if env.Data.ID == ui.VarPopupEnvID {
				updated := false
				for i, v := range env.Data.Vars {
					if v.Key == ui.VarPopupName {
						env.Data.Vars[i].Value = ui.VarPopupEditor.Text()
						updated = true
						break
					}
				}
				if !updated {
					env.Data.Vars = append(env.Data.Vars, EnvVar{
						Key:     ui.VarPopupName,
						Value:   ui.VarPopupEditor.Text(),
						Enabled: true,
					})
				}
				SaveEnvironment(env.Data)
				ui.activeEnvDirty = true
				break
			}
		}
	}
}
