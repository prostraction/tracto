# Project Tracto

### Prep

- Install CGO

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
go run ./...
```

### Build

Windows

```
go build -ldflags="-s -w -H=windowsgui" -o bin\tracto.exe cmd\main.go && upx --best --lzma bin\tracto.exe
```