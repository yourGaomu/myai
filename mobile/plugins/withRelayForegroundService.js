const fs = require("fs");
const path = require("path");

const {
  withAndroidManifest,
  withDangerousMod,
  withMainApplication,
} = require("@expo/config-plugins");

const SERVICE_PACKAGE = "com.myai.mobile.relay";
const SERVICE_CLASS = "RelayForegroundService";

function withRelayForegroundService(config) {
  config = withAndroidManifest(config, (config) => {
    const manifest = config.modResults.manifest;
    const permissions = manifest["uses-permission"] || [];
    const permissionNames = new Set(permissions.map((permission) => permission?.$?.["android:name"]));
    for (const name of [
      "android.permission.FOREGROUND_SERVICE",
      "android.permission.FOREGROUND_SERVICE_REMOTE_MESSAGING",
      "android.permission.POST_NOTIFICATIONS",
      "android.permission.ACCESS_NETWORK_STATE",
    ]) {
      if (!permissionNames.has(name)) {
        permissions.push({ $: { "android:name": name } });
      }
    }
    manifest["uses-permission"] = permissions;

    const application = manifest.application?.[0];
    if (!application) {
      throw new Error("Relay foreground service requires an Android application manifest.");
    }
    application.service = application.service || [];
    const existing = application.service.find(
      (service) => service?.$?.["android:name"] === `.${SERVICE_PACKAGE.split(".").pop()}.${SERVICE_CLASS}`
        || service?.$?.["android:name"] === `${SERVICE_PACKAGE}.${SERVICE_CLASS}`,
    );
    const service = {
      $: {
        "android:name": `.${SERVICE_PACKAGE.split(".").pop()}.${SERVICE_CLASS}`,
        "android:enabled": "true",
        "android:exported": "false",
        "android:foregroundServiceType": "remoteMessaging",
      },
    };
    if (existing) {
      Object.assign(existing.$, service.$);
    } else {
      application.service.push(service);
    }
    return config;
  });

  config = withMainApplication(config, (config) => {
    let contents = config.modResults.contents;
    if (!contents.includes("RelayForegroundServicePackage")) {
      contents = contents.replace(
        "import expo.modules.ApplicationLifecycleDispatcher",
        "import com.myai.mobile.relay.RelayForegroundServicePackage\n\nimport expo.modules.ApplicationLifecycleDispatcher",
      );
      contents = contents.replace(
        "// add(MyReactNativePackage())",
        "// add(MyReactNativePackage())\n          add(RelayForegroundServicePackage())",
      );
    }
    config.modResults.contents = contents;
    return config;
  });

  return withDangerousMod(config, ["android", async (config) => {
    const sourceRoot = path.join(config.modRequest.projectRoot, "plugins", "relay-foreground-service");
    const targetRoot = path.join(
      config.modRequest.platformProjectRoot,
      "app",
      "src",
      "main",
      "java",
      ...SERVICE_PACKAGE.split("."),
    );
    fs.mkdirSync(targetRoot, { recursive: true });
    for (const fileName of ["RelayForegroundService.java", "RelayForegroundServiceModule.java", "RelayForegroundServicePackage.java"]) {
      fs.copyFileSync(path.join(sourceRoot, fileName), path.join(targetRoot, fileName));
    }
    const drawableRoot = path.join(
      config.modRequest.platformProjectRoot,
      "app",
      "src",
      "main",
      "res",
      "drawable",
    );
    fs.mkdirSync(drawableRoot, { recursive: true });
    fs.copyFileSync(
      path.join(sourceRoot, "relay_notification.xml"),
      path.join(drawableRoot, "relay_notification.xml"),
    );
    return config;
  }]);
}

module.exports = withRelayForegroundService;
