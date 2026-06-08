# MyAI Mobile

Expo React Native client for the MyAI Relay.

## Run

```powershell
cd D:\Go_All\myai\mobile
npm start
```

Then open the app with Expo Go or an Android emulator.

## Relay URL

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
3. Enter the Agent bind code in the mobile app.
4. Tap `Pair`.
5. Tap `Connect`.
6. Chat, switch sessions, and approve or deny tool permissions.
7. Open `Files` to browse and preview files from the PC Agent workspace.
8. Open `Changes` to inspect Git changes, preview diffs, and jump to the changed file.

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

## Local Android Build

Cloud build is the easiest path. Local builds need Android Studio, JDK, Android SDK, and more environment setup.

```powershell
cd D:\Go_All\myai\mobile
eas build -p android --profile preview --local
```
