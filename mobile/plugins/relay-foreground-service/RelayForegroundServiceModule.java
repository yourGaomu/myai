package com.myai.mobile.relay;

import android.content.Context;
import android.content.Intent;

import androidx.core.content.ContextCompat;

import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;

public final class RelayForegroundServiceModule extends ReactContextBaseJavaModule {
  public RelayForegroundServiceModule(ReactApplicationContext reactContext) {
    super(reactContext);
  }

  @Override
  public String getName() {
    return "RelayForegroundService";
  }

  @ReactMethod
  public void start(Promise promise) {
    try {
      Context context = getReactApplicationContext();
      Intent intent = new Intent(context, RelayForegroundService.class)
        .setAction(RelayForegroundService.ACTION_START);
      ContextCompat.startForegroundService(context, intent);
      promise.resolve(true);
    } catch (Throwable error) {
      promise.reject("RELAY_FOREGROUND_SERVICE_START_FAILED", error);
    }
  }

  @ReactMethod
  public void stop(Promise promise) {
    try {
      Context context = getReactApplicationContext();
      context.stopService(new Intent(context, RelayForegroundService.class));
      promise.resolve(true);
    } catch (Throwable error) {
      promise.reject("RELAY_FOREGROUND_SERVICE_STOP_FAILED", error);
    }
  }

  @ReactMethod
  public void updateStatus(String status, Promise promise) {
    try {
      RelayForegroundService.updateStatus(getReactApplicationContext(), status);
      promise.resolve(true);
    } catch (Throwable error) {
      promise.reject("RELAY_FOREGROUND_SERVICE_STATUS_UPDATE_FAILED", error);
    }
  }

  @ReactMethod
  public void isRunning(Promise promise) {
    promise.resolve(RelayForegroundService.isRunning());
  }
}
