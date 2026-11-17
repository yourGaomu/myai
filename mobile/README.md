# MyAI Mobile

Expo React Native client for the MyAI Relay.

## Run

```powershell
cd D:\Go_All\myai\mobile
npm start
```

Then open the app with Expo Go or an Android emulator.

## Relay URL

The current deployed services use:

```text
https://relay.mikasa.wiki
https://assets.mikasa.wiki
```

These are also the defaults in the app. On first launch the app shows the
pairing screen: enter the six-digit bind code printed by the PC Agent and tap
`配对并连接`. Pairing fills the correct user/device identity and exchanges a
client token, then the app connects automatically; the bind code is not used as
the WebSocket token. To pair again or change the relay address, open
sidebar → 设置 → 连接与配对.

If the Relay runs on the same PC and you test in an Android emulator, use:

```text
http://10.0.2.2:18080
```

If you test on a real phone, use the PC LAN IP or your deployed server, for example:

```text
http://192.168.1.23:18080
```

Do not use `127.0.0.1` on a real phone unless the Relay also runs on that phone.

## Flow

1. Start Relay.
2. Start PC Agent.
3. Enter the Agent bind code on the pairing screen and tap `配对并连接`.
4. Chat, approve or deny tool permissions, and tap the title to switch model, mode, and tool permissions.
5. Open the sidebar to switch sessions or open `文件`, `变更`, `知识库与记忆`, and `设置`.
6. In `文件`, browse and preview files from the PC Agent workspace and add them to the chat.
7. In `变更`, inspect changes compared with the SQLite workspace history baseline, preview diffs, open files, and revert restorable files.

## Build Android APK

Install EAS CLI:

```powershell
npm install -g eas-cli
```

Login:

```powershell
eas login
```

Build an installable APK:

```powershell
cd D:\Go_All\myai\mobile
npm run build:android:apk
```

After the build finishes, Expo will print a download link for the `.apk`.
Download it, send it to your phone, and install it.

For a Google Play release build, use:

```powershell
npm run build:android:aab
```

## OTA Updates

OTA updates are enabled for JavaScript, UI, and bundled assets. The first APK/AAB built with this configuration must be installed once; later compatible changes can be published without rebuilding the APK.

Publish an internal preview update:

```powershell
cd D:\Go_All\myai\mobile
npm run update:android:preview -- --message "Describe the change"
```

Publish a production update:

```powershell
npm run update:android:production -- --message "Describe the change"
```

The app checks for updates shortly after launch and when returning to the foreground. It downloads an available update in the background and asks the user to restart. Native Android changes still require a new APK/AAB and an incremented app version.

## Local Android Build

Cloud build is the easiest path. Local builds need Android Studio, JDK, Android SDK, and more environment setup.

```powershell
cd D:\Go_All\myai\mobile
eas build -p android --profile preview --local
```
