# Project Tracto

### Prep

> [!WARNING]
> lib fork required for compilation.

1. Fork lib

```
go mod tidy
go mod vendor
```

2. Add these lines into vendor\gioui.org\widget\editor.go:

```
func (e *Editor) GetScrollY() int {
	e.initBuffer()
	return e.text.ScrollOff().Y
}

func (e *Editor) SetScrollY(y int) {
	e.initBuffer()
	current := e.text.ScrollOff().Y
	e.text.ScrollRel(0, y-current)
}

func (e *Editor) GetScrollBounds() image.Rectangle {
	e.initBuffer()
	return e.text.ScrollBounds()
}

```

### Windows Prep Build (CGO install):

```
Invoke-WebRequest -Uri "https://repo.msys2.org/distrib/x86_64/msys2-x86_64-20251213.exe" -OutFile "msys2-installer.exe"
Start-Process -FilePath ".\msys2-installer.exe" -ArgumentList "in,--confirm-command,--accept-messages,--root=C:/msys64" -Wait
C:\msys64\usr\bin\bash.exe -lc "pacman -S --noconfirm mingw-w64-x86_64-toolchain"
$env:PATH += ";C:\msys64\mingw64\bin"
$env:CGO_ENABLED="1"
```

### Linux Prep Build:

Ensure you have CGO_ENABLED="1", install gcc if nesessary.

### Lauch:

```
go run -mod=vendor ./...
```