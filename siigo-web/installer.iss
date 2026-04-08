; Siigo Web - Inno Setup Installer Script
; Download Inno Setup from: https://jrsoftware.org/isdl.php
; Open this file in Inno Setup Compiler and click Build

[Setup]
AppId={{B8F3E2A1-5C4D-4E6F-A7B9-1234567890AB}
AppName=Siigo Web
AppVersion=1.0
AppPublisher=Siigo Middleware
AppPublisherURL=https://github.com/lordmacu/siigo-bridge
DefaultDirName={localappdata}\SiigoWeb
DefaultGroupName=Siigo Web
OutputDir=installer_output
OutputBaseFilename=SiigoWeb-Setup
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=lowest
SetupIconFile=siigobridge.ico
WizardImageFile=wizard_big.bmp
WizardSmallImageFile=wizard_small.bmp
DisableProgramGroupPage=yes
UninstallDisplayName=Siigo Web

[Files]
; Main executable (built with build.bat)
Source: "siigo-web.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "siigobridge.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "README.txt"; DestDir: "{app}"; Flags: ignoreversion isreadme

[Icons]
; Desktop shortcut
Name: "{autodesktop}\Siigo Bridge"; Filename: "{app}\siigo-web.exe"; WorkingDir: "{app}"; IconFilename: "{app}\siigobridge.ico"; Comment: "Abrir Siigo Bridge"
; Start Menu shortcut
Name: "{group}\Siigo Bridge"; Filename: "{app}\siigo-web.exe"; WorkingDir: "{app}"; IconFilename: "{app}\siigobridge.ico"; Comment: "Abrir Siigo Bridge"
; Start Menu uninstall
Name: "{group}\Desinstalar Siigo Web"; Filename: "{uninstallexe}"

[Run]
; Launch after install
Filename: "{app}\siigo-web.exe"; Description: "Iniciar Siigo Web"; Flags: nowait postinstall skipifsilent
; README removed - opens via web panel instead

[Tasks]
Name: "autostart"; Description: "Iniciar Siigo Web automaticamente al encender Windows"; GroupDescription: "Opciones adicionales:"

[Registry]
; Auto-start with Windows (only if user checked the task)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "SiigoWeb"; ValueData: """{app}\siigo-web.exe"""; Flags: uninsdeletevalue; Tasks: autostart

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
  PortPage: TInputQueryWizardPage;
  DataPathPage: TInputDirWizardPage;
  FirewallOpened: Boolean;

// Escape backslashes for JSON strings: C:\SIIWI02 -> C:\\SIIWI02
function EscapeJSON(const S: String): String;
begin
  Result := S;
  StringChangeEx(Result, '\', '\\', True);
  StringChangeEx(Result, '"', '\"', True);
end;

// Check if a file exists (any of the known ISAM files)
function FileExistsInDir(const Dir, FileName: String): Boolean;
begin
  Result := FileExists(AddBackslash(Dir) + FileName);
end;

// Count how many known ISAM files exist in the directory
function CountISAMFiles(const Dir: String): Integer;
var
  KnownFiles: array[0..7] of String;
  I: Integer;
begin
  KnownFiles[0] := 'Z17';
  KnownFiles[1] := 'Z06';
  KnownFiles[2] := 'Z49';
  KnownFiles[3] := 'ZDANE';
  KnownFiles[4] := 'ZICA';
  KnownFiles[5] := 'ZPILA';
  KnownFiles[6] := 'Z06A';
  KnownFiles[7] := 'Z279CP';
  Result := 0;
  for I := 0 to 7 do
  begin
    if FileExistsInDir(Dir, KnownFiles[I]) then
      Result := Result + 1;
  end;
end;

// Open firewall port using netsh (requires admin)
function OpenFirewallPort(Port: String): Boolean;
var
  ResultCode: Integer;
begin
  // First remove existing rule (ignore errors)
  Exec('netsh', 'advfirewall firewall delete rule name="Siigo Web"', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  // Add new rule
  Result := Exec('netsh',
    'advfirewall firewall add rule name="Siigo Web" dir=in action=allow protocol=TCP localport=' + Port,
    '', SW_HIDE, ewWaitUntilTerminated, ResultCode) and (ResultCode = 0);
end;

// Remove firewall rule on uninstall
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  ResultCode: Integer;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    Exec('netsh', 'advfirewall firewall delete rule name="Siigo Web"', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
end;

procedure InitializeWizard;
begin
  FirewallOpened := False;

  // Page 1: Credentials
  CredentialsPage := CreateInputQueryPage(wpSelectDir,
    'Configuracion de Acceso',
    'Ingrese las credenciales para acceder al panel web',
    'Estas seran las credenciales de administrador para iniciar sesion en Siigo Web.');
  CredentialsPage.Add('Usuario:', False);
  CredentialsPage.Add('Contrasena:', True);
  CredentialsPage.Values[0] := 'admin';
  CredentialsPage.Values[1] := '';

  // Page 2: Port configuration
  PortPage := CreateInputQueryPage(CredentialsPage.ID,
    'Configuracion de Puerto',
    'Defina el puerto para el servidor web',
    'Este puerto se usara para acceso local (localhost), por red LAN y para el tunel de Cloudflare.' + #13#10 +
    'El firewall de Windows se configurara automaticamente para permitir conexiones en este puerto.' + #13#10 + #13#10 +
    'Valor recomendado: 3210. Solo cambielo si ese puerto ya esta en uso.');
  PortPage.Add('Puerto:', False);
  PortPage.Values[0] := '3210';

  // Page 3: Siigo data path
  DataPathPage := CreateInputDirPage(PortPage.ID,
    'Ruta de Datos Siigo',
    'Seleccione la carpeta donde estan los archivos de Siigo Pyme',
    'El instalador verificara que la carpeta contenga archivos ISAM de Siigo (Z17, Z06, Z49, etc.).' + #13#10 +
    'Si la carpeta no contiene archivos reconocidos, no podra continuar.',
    False, '');
  DataPathPage.Add('');
  DataPathPage.Values[0] := 'C:\SIIWI02';
end;

function IsValidPort(const S: String): Boolean;
var
  Port: Integer;
begin
  Result := False;
  Port := StrToIntDef(S, -1);
  if (Port >= 1024) and (Port <= 65535) then
    Result := True;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  Port: String;
  DataPath: String;
  FileCount: Integer;
begin
  Result := True;

  // Validate credentials
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

  // Validate port and open firewall
  if CurPageID = PortPage.ID then
  begin
    Port := Trim(PortPage.Values[0]);
    if not IsValidPort(Port) then
    begin
      MsgBox('El puerto debe ser un numero entre 1024 y 65535.', mbError, MB_OK);
      Result := False;
      Exit;
    end;

    // Try to open firewall
    if OpenFirewallPort(Port) then
    begin
      FirewallOpened := True;
      MsgBox('Puerto ' + Port + ' abierto en el firewall de Windows correctamente.' + #13#10 + #13#10 +
        'Podra acceder al panel desde otros equipos en la red local.',
        mbInformation, MB_OK);
    end
    else
    begin
      if MsgBox('No se pudo abrir el puerto ' + Port + ' en el firewall.' + #13#10 + #13#10 +
        'El servidor funcionara en localhost pero podria no ser accesible desde otros equipos en la red.' + #13#10 + #13#10 +
        'Desea continuar de todas formas?',
        mbConfirmation, MB_YESNO) = IDNO then
      begin
        Result := False;
        Exit;
      end;
    end;
  end;

  // Validate Siigo data path
  if CurPageID = DataPathPage.ID then
  begin
    DataPath := DataPathPage.Values[0];

    // Check directory exists
    if not DirExists(DataPath) then
    begin
      MsgBox('La carpeta "' + DataPath + '" no existe.' + #13#10 + #13#10 +
        'Verifique la ruta e intente de nuevo.',
        mbError, MB_OK);
      Result := False;
      Exit;
    end;

    // Check for ISAM files
    FileCount := CountISAMFiles(DataPath);
    if FileCount = 0 then
    begin
      MsgBox('No se encontraron archivos ISAM de Siigo en "' + DataPath + '".' + #13#10 + #13#10 +
        'Se buscaron archivos como Z17, Z06, Z49, ZDANE, ZICA, ZPILA, etc.' + #13#10 +
        'Verifique que esta sea la carpeta correcta de datos de Siigo Pyme.',
        mbError, MB_OK);
      Result := False;
      Exit;
    end;

    if FileCount < 3 then
    begin
      if MsgBox('Solo se encontraron ' + IntToStr(FileCount) + ' archivo(s) ISAM en "' + DataPath + '".' + #13#10 + #13#10 +
        'Normalmente Siigo tiene 6 o mas archivos base. Es posible que esta no sea la carpeta correcta.' + #13#10 + #13#10 +
        'Desea continuar de todas formas?',
        mbConfirmation, MB_YESNO) = IDNO then
      begin
        Result := False;
        Exit;
      end;
    end
    else
    begin
      MsgBox('Se encontraron ' + IntToStr(FileCount) + ' archivos ISAM de Siigo. La carpeta es valida.',
        mbInformation, MB_OK);
    end;
  end;
end;

procedure GenerateConfigJSON;
var
  ConfigFile: String;
  Lines: TStringList;
  EscUser, EscPass, EscPath, Port: String;
begin
  ConfigFile := ExpandConstant('{app}\config.json');

  // Escape special characters for valid JSON
  EscUser := EscapeJSON(CredentialsPage.Values[0]);
  EscPass := EscapeJSON(CredentialsPage.Values[1]);
  EscPath := EscapeJSON(DataPathPage.Values[0]);
  Port := Trim(PortPage.Values[0]);

  Lines := TStringList.Create;
  try
    Lines.Add('{');
    Lines.Add('  "auth": {');
    Lines.Add('    "username": "' + EscUser + '",');
    Lines.Add('    "password": "' + EscPass + '"');
    Lines.Add('  },');
    Lines.Add('  "server": {');
    Lines.Add('    "port": "' + Port + '"');
    Lines.Add('  },');
    Lines.Add('  "siigo": {');
    Lines.Add('    "data_path": "' + EscPath + '"');
    Lines.Add('  },');
    Lines.Add('  "finearom": {');
    Lines.Add('    "base_url": "",');
    Lines.Add('    "email": "",');
    Lines.Add('    "password": ""');
    Lines.Add('  },');
    Lines.Add('  "sync": {');
    Lines.Add('    "interval_seconds": 60,');
    Lines.Add('    "send_interval_seconds": 30,');
    Lines.Add('    "batch_size": 50,');
    Lines.Add('    "batch_delay_ms": 500,');
    Lines.Add('    "max_retries": 3,');
    Lines.Add('    "retry_delay_seconds": 30,');
    Lines.Add('    "files": ["Z17", "Z04", "Z49"],');
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
    Lines.Add('  },');
    Lines.Add('  "webhooks": {');
    Lines.Add('    "enabled": false,');
    Lines.Add('    "hooks": []');
    Lines.Add('  },');
    Lines.Add('  "setup_complete": false');
    Lines.Add('}');
    Lines.SaveToFile(ConfigFile);
  finally
    Lines.Free;
  end;
end;

// Check if already installed and offer to uninstall first
function InitializeSetup: Boolean;
var
  UninstallKey: String;
  UninstallString: String;
  ResultCode: Integer;
begin
  Result := True;
  UninstallKey := 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{B8F3E2A1-5C4D-4E6F-A7B9-1234567890AB}_is1';

  if RegQueryStringValue(HKCU, UninstallKey, 'UninstallString', UninstallString) or
     RegQueryStringValue(HKLM, UninstallKey, 'UninstallString', UninstallString) then
  begin
    case MsgBox('Siigo Web ya esta instalado.' + #13#10 + #13#10 +
      'Seleccione una opcion:' + #13#10 +
      '  SI = Desinstalar version anterior y continuar con la instalacion' + #13#10 +
      '  NO = Actualizar sin desinstalar (conserva datos)' + #13#10 +
      '  CANCELAR = No hacer nada',
      mbConfirmation, MB_YESNOCANCEL) of
      IDYES:
        begin
          // Uninstall first
          Exec('taskkill', '/F /IM siigo-web.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
          if not Exec(RemoveQuotes(UninstallString), '/SILENT', '', SW_SHOWNORMAL, ewWaitUntilTerminated, ResultCode) then
            MsgBox('No se pudo desinstalar. Intente manualmente.', mbError, MB_OK);
        end;
      IDNO:
        begin
          // Just kill and continue (update)
          Exec('taskkill', '/F /IM siigo-web.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
        end;
      IDCANCEL:
        Result := False;
    end;
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
