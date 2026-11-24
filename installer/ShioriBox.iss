; ShioriBox installer script for Inno Setup 6.3 or newer (https://jrsoftware.org/isinfo.php)
; Build it by double-clicking build-installer.bat. That makes TWO installers in the "output" folder:
;   ShioriBox-Setup-<version>.exe   full installer for a new installation
;   ShioriBox-Update-<version>.exe  update for an existing installation (keeps your library; no questions asked)
; The update one is this same script compiled with /DUPDATE (see build-installer.bat).

#define AppName "ShioriBox"
#define AppVersion "1.22.0"
#define AppExe "ShioriBox.exe"
#ifdef UPDATE
  #define Kind "Update"
#else
  #define Kind "Setup"
#endif

[Setup]
; Never change the AppId: it is how Windows recognises upgrades of the same app.
AppId={{B7C1E5A2-4D3F-4C8E-9A61-5F0E2D7B9C14}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppName}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
; Installs just for you (no admin prompt). The wizard lets you choose "for all users" instead.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputDir=output
OutputBaseFilename=ShioriBox-{#Kind}-{#AppVersion}
SetupMutex=ShioriBoxInstallerMutex
SetupIconFile=..\source\shioribox-app\icon.ico
UninstallDisplayIcon={app}\{#AppExe}
UninstallDisplayName={#AppName}
Compression=lzma2/ultra
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
; Close a running ShioriBox during install/upgrade instead of failing on a locked file.
CloseApplications=yes
RestartApplications=no
#ifdef UPDATE
; Update: reuse the folder and install mode of the existing installation and skip every question.
UsePreviousAppDir=yes
UsePreviousPrivileges=yes
DisableWelcomePage=yes
DisableDirPage=yes
DisableReadyPage=yes

[Messages]
SetupAppTitle=ShioriBox Update
SetupWindowTitle=Update - %1
#endif

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

#ifndef UPDATE
[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
#endif

[Files]
Source: "..\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExe}"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"
#ifndef UPDATE
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon
#endif

[Run]
Filename: "{app}\{#AppExe}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent

; Your library lives in a "data" folder next to ShioriBox.exe (or in %AppData%\ShioriBox when the install
; folder is read-only, e.g. Program Files). Neither is installed by this script, so upgrades keep your
; library. The uninstaller asks before deleting it (see [Code]).

[Code]
const
#ifdef UPDATE
  IsUpdate = True;
#else
  IsUpdate = False;
#endif
  UninstKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{B7C1E5A2-4D3F-4C8E-9A61-5F0E2D7B9C14}_is1';
  WebView2Key = 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';

function WebView2Installed: Boolean;
var
  v: String;
begin
  Result := (RegQueryStringValue(HKLM32, WebView2Key, 'pv', v) and (v <> '') and (v <> '0.0.0.0'))
         or (RegQueryStringValue(HKCU, WebView2Key, 'pv', v) and (v <> '') and (v <> '0.0.0.0'));
end;

function ExistingInstall: Boolean;
var
  s: String;
begin
  Result := (RegQueryStringValue(HKCU, UninstKey, 'InstallLocation', s) and (s <> ''))
         or (RegQueryStringValue(HKLM, UninstKey, 'InstallLocation', s) and (s <> ''));
end;

function InitializeSetup: Boolean;
begin
  Result := True;
  if IsUpdate then
  begin
    if not ExistingInstall then
    begin
      MsgBox('ShioriBox is not installed on this PC, so there is nothing to update.' + #13#10 +
             'Run the full installer (ShioriBox-Setup) instead.', mbError, MB_OK);
      Result := False;
    end;
  end
  else if (not WebView2Installed) and (not WizardSilent) then
    MsgBox('ShioriBox needs the Microsoft Edge WebView2 runtime, which was not found on this PC.' + #13#10 +
           'It is included with Windows 11 and recent Windows 10 updates. If ShioriBox does not open after' + #13#10 +
           'installing, get the free "Evergreen Runtime" from microsoft.com/edge/webview2 and run ShioriBox again.',
           mbInformation, MB_OK);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  dataA, dataB: String;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    dataA := ExpandConstant('{app}\data');
    dataB := ExpandConstant('{userappdata}\ShioriBox');
    if (DirExists(dataA) or DirExists(dataB)) and (not UninstallSilent) then
      if MsgBox('Also delete your ShioriBox library, covers, guides and settings?' + #13#10#13#10 +
                'Choose No to keep them (for example to reinstall later).',
                mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
      begin
        DelTree(dataA, True, True, True);
        DelTree(dataB, True, True, True);
        RemoveDir(ExpandConstant('{app}'));
      end;
  end;
end;
