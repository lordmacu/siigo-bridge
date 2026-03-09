; Siigo Web - Inno Setup Installer Script
; Download Inno Setup from: https://jrsoftware.org/isdl.php
; Open this file in Inno Setup Compiler and click Build

[Setup]
AppName=Siigo Web
AppVersion=1.0
AppPublisher=Siigo Middleware
AppPublisherURL=https://finearom.co
DefaultDirName={autopf}\SiigoWeb
DefaultGroupName=Siigo Web
OutputDir=installer_output
OutputBaseFilename=SiigoWeb-Setup
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=lowest
; Uncomment next line if you have a custom icon file:
; SetupIconFile=siigo-web.ico
DisableProgramGroupPage=yes
UninstallDisplayName=Siigo Web

[Files]
; Main executable (built with build.bat)
Source: "siigo-web.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; Desktop shortcut
Name: "{autodesktop}\Siigo Web"; Filename: "{app}\siigo-web.exe"; WorkingDir: "{app}"; Comment: "Abrir Siigo Web"
; Start Menu shortcut
Name: "{group}\Siigo Web"; Filename: "{app}\siigo-web.exe"; WorkingDir: "{app}"; Comment: "Abrir Siigo Web"
; Start Menu uninstall
Name: "{group}\Desinstalar Siigo Web"; Filename: "{uninstallexe}"

[Run]
; Launch after install
Filename: "{app}\siigo-web.exe"; Description: "Iniciar Siigo Web"; Flags: nowait postinstall skipifsilent

[Registry]
; Auto-start with Windows (current user, no admin needed)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "SiigoWeb"; ValueData: """{app}\siigo-web.exe"""; Flags: uninsdeletevalue

[UninstallDelete]
; Clean up all generated files and the app folder on uninstall
Type: files; Name: "{app}\config.json"
Type: files; Name: "{app}\siigo_web.db"
Type: files; Name: "{app}\siigo_web.db-journal"
Type: files; Name: "{app}\siigo_web.db-wal"
Type: files; Name: "{app}\siigo_web.db-shm"
Type: files; Name: "{app}\sync_state.json"
Type: filesandordirs; Name: "{app}\logs"
Type: dirifempty; Name: "{app}"

[UninstallRun]
; Kill running instance before uninstall
Filename: "taskkill"; Parameters: "/F /IM siigo-web.exe"; Flags: runhidden; RunOnceId: "KillSiigoWeb"

[Code]
var
  CredentialsPage: TInputQueryWizardPage;
  DataPathPage: TInputDirWizardPage;

// Escape backslashes for JSON strings: C:\DEMOS01\ -> C:\\DEMOS01\\
function EscapeJSON(const S: String): String;
begin
  Result := S;
  StringChangeEx(Result, '\', '\\', True);
  StringChangeEx(Result, '"', '\"', True);
end;

procedure InitializeWizard;
begin
  // Page 1: Credentials
  CredentialsPage := CreateInputQueryPage(wpSelectDir,
    'Configuracion de Acceso',
    'Ingrese las credenciales para acceder al panel web',
    'Estas seran las credenciales de administrador para iniciar sesion en Siigo Web.');
  CredentialsPage.Add('Usuario:', False);
  CredentialsPage.Add('Contrasena:', True);
  CredentialsPage.Values[0] := 'admin';
  CredentialsPage.Values[1] := '';

  // Page 2: Siigo data path
  DataPathPage := CreateInputDirPage(CredentialsPage.ID,
    'Ruta de Datos Siigo',
    'Seleccione la carpeta donde estan los archivos de Siigo Pyme',
    'El instalador buscara los archivos ISAM (Z17, Z04, Z49, etc.) en esta ruta.',
    False, '');
  DataPathPage.Add('');
  DataPathPage.Values[0] := 'C:\DEMOS01\';
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = CredentialsPage.ID then
  begin
    if CredentialsPage.Values[0] = '' then
    begin
      MsgBox('Debe ingresar un nombre de usuario.', mbError, MB_OK);
      Result := False;
      Exit;
    end;
    if Length(CredentialsPage.Values[1]) < 4 then
    begin
      MsgBox('La contrasena debe tener al menos 4 caracteres.', mbError, MB_OK);
      Result := False;
      Exit;
    end;
  end;
end;

procedure GenerateConfigJSON;
var
  ConfigFile: String;
  Lines: TStringList;
  EscUser, EscPass, EscPath: String;
begin
  ConfigFile := ExpandConstant('{app}\config.json');

  // Escape special characters for valid JSON
  EscUser := EscapeJSON(CredentialsPage.Values[0]);
  EscPass := EscapeJSON(CredentialsPage.Values[1]);
  EscPath := EscapeJSON(DataPathPage.Values[0]);

  Lines := TStringList.Create;
  try
    Lines.Add('{');
    Lines.Add('  "auth": {');
    Lines.Add('    "username": "' + EscUser + '",');
    Lines.Add('    "password": "' + EscPass + '"');
    Lines.Add('  },');
    Lines.Add('  "server": {');
    Lines.Add('    "port": "3210"');
    Lines.Add('  },');
    Lines.Add('  "siigo": {');
    Lines.Add('    "data_path": "' + EscPath + '"');
    Lines.Add('  },');
    Lines.Add('  "finearom": {');
    Lines.Add('    "base_url": "https://ordenes.finearom.co/api",');
    Lines.Add('    "email": "siigo-sync@finearom.com",');
    Lines.Add('    "password": ""');
    Lines.Add('  },');
    Lines.Add('  "sync": {');
    Lines.Add('    "interval_seconds": 60,');
    Lines.Add('    "send_interval_seconds": 30,');
    Lines.Add('    "batch_size": 50,');
    Lines.Add('    "batch_delay_ms": 500,');
    Lines.Add('    "max_retries": 3,');
    Lines.Add('    "retry_delay_seconds": 30,');
    Lines.Add('    "files": ["Z17", "Z04", "Z49", "Z092024"],');
    Lines.Add('    "state_path": "sync_state.json"');
    Lines.Add('  },');
    Lines.Add('  "public_api": {');
    Lines.Add('    "enabled": true,');
    Lines.Add('    "jwt_required": true,');
    Lines.Add('    "api_key": "change-me-to-a-secure-key",');
    Lines.Add('    "jwt_secret": "change-me-to-a-random-secret"');
    Lines.Add('  },');
    Lines.Add('  "telegram": {');
    Lines.Add('    "enabled": false,');
    Lines.Add('    "bot_token": "",');
    Lines.Add('    "chat_id": 0,');
    Lines.Add('    "exec_pin": "2337"');
    Lines.Add('  }');
    Lines.Add('}');
    Lines.SaveToFile(ConfigFile);
  finally
    Lines.Free;
  end;
end;

// Kill any running instance before installing/updating
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  Exec('taskkill', '/F /IM siigo-web.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Result := '';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    GenerateConfigJSON;
  end;
end;
