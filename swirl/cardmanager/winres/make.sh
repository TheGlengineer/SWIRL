#!/bin/sh
# Builds rsrc_windows_amd64.syso (icon, version info, manifest). Run after changing the version.
# usage: winres/make.sh 2.7.0
set -e
cd "$(dirname "$0")"
V="$1"; IFS=. read a b c <<END
$V
END
c=${c:-0}
cat > app.rc <<RC
#include <winver.h>
1 ICON "../assets/app.ico"
1 24 "app.manifest"
VS_VERSION_INFO VERSIONINFO
FILEVERSION $a,$b,$c,0
PRODUCTVERSION $a,$b,$c,0
FILEOS VOS_NT_WINDOWS32
FILETYPE VFT_APP
BEGIN
  BLOCK "StringFileInfo"
  BEGIN
    BLOCK "040904B0"
    BEGIN
      VALUE "CompanyName", "Glen Huszar"
      VALUE "FileDescription", "SWIRL Card Manager"
      VALUE "FileVersion", "$V"
      VALUE "InternalName", "SWIRL Card Manager"
      VALUE "LegalCopyright", "Glen Huszar (github.com/TheGlengineer). Built on openMenu."
      VALUE "OriginalFilename", "SWIRL Card Manager.exe"
      VALUE "ProductName", "SWIRL Card Manager"
      VALUE "ProductVersion", "$V"
    END
  END
  BLOCK "VarFileInfo"
  BEGIN
    VALUE "Translation", 0x409, 1200
  END
END
RC
x86_64-w64-mingw32-windres -O coff -i app.rc -o ../rsrc_windows_amd64.syso
echo "wrote rsrc_windows_amd64.syso for $V"
