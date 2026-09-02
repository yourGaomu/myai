package com.myai.mobile.relay;

import android.content.Context;
import android.content.Intent;

import androidx.core.content.ContextCompat;

import com.facebook.react.bridge.Arguments;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.WritableArray;
import com.facebook.react.bridge.WritableMap;
import com.facebook.react.modules.core.DeviceEventManagerModule;

import java.util.List;

public final class RelayForegroundServiceModule extends ReactContextBaseJavaModule {
  public static final String STATE_EVENT = "RelayForegroundServiceState";
  public static final String MESSAGE_EVENT = "RelayForegroundServiceMessage";

  private final ReactApplicationContext reactContext;

  public RelayForegroundServiceModule(ReactApplicationContext reactContext) {
    super(reactContext);
    this.reactContext = reactContext;
    RelayForegroundService.setEventSink(new RelayForegroundService.EventSink() {
      @Override
      public void onState(String state, String status, String error) {
        emitState(state, status, error);
      }

      @Override
      public void onMessagesAvailable() {
        emitMessageAvailable();
      }
    });
  }

  @Override
  public String getName() {
    return "RelayForegroundService";
  }

  @ReactMethod
  public void start(String websocketURL, String userID, String deviceID, String clientToken, Promise promise) {
    try {
      Context context = getReactApplicationContext();
      Intent intent = new Intent(context, RelayForegroundService.class)
        .setAction(RelayForegroundService.ACTION_START)
        .putExtra(RelayForegroundService.EXTRA_WEBSOCKET_URL, websocketURL)
        .putExtra(RelayForegroundService.EXTRA_USER_ID, userID)
        .putExtra(RelayForegroundService.EXTRA_DEVICE_ID, deviceID)
        .putExtra(RelayForegroundService.EXTRA_CLIENT_TOKEN, clientToken);
      ContextCompat.startForegroundService(context, intent);
      promise.resolve(true);
    } catch (Throwable error) {
      promise.reject("RELAY_FOREGROUND_SERVICE_START_FAILED", error);
    }
  }

  @ReactMethod
  public void stop(Promise promise) {
    try {
      RelayForegroundService.requestStop(getReactApplicationContext());
      promise.resolve(true);
    } catch (Throwable error) {
      promise.reject("RELAY_FOREGROUND_SERVICE_STOP_FAILED", error);
    }
  }

  @ReactMethod
  public void send(String message, Promise promise) {
    try {
      promise.resolve(RelayForegroundService.sendMessage(message));
    } catch (Throwable error) {
      promise.reject("RELAY_FOREGROUND_SERVICE_SEND_FAILED", error);
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

  @ReactMethod
  public void getState(Promise promise) {
    WritableMap result = Arguments.createMap();
    result.putString("state", RelayForegroundService.getState());
    result.putString("status", RelayForegroundService.getStatusText());
    result.putString("error", RelayForegroundService.getLastError());
    promise.resolve(result);
  }

  @ReactMethod
  public void drainMessages(Promise promise) {
    List<String> messages = RelayForegroundService.drainMessages();
    WritableArray result = Arguments.createArray();
    for (String message : messages) {
      result.pushString(message);
    }
    promise.resolve(result);
  }

  // React Native requires these no-op listener methods when a native module emits
  // DeviceEventEmitter events, otherwise it warns during bridge initialization.
  @ReactMethod
  public void addListener(String eventName) {
    // no-op
  }

  @ReactMethod
  public void removeListeners(double count) {
    // no-op
  }

  private void emitState(String state, String status, String error) {
    if (!reactContext.hasActiveReactInstance()) {
      return;
    }
    WritableMap payload = Arguments.createMap();
    payload.putString("state", state);
    payload.putString("status", status);
    payload.putString("error", error);
    reactContext.runOnJSQueueThread(() -> reactContext
      .getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter.class)
      .emit(STATE_EVENT, payload));
  }

  private void emitMessageAvailable() {
    if (!reactContext.hasActiveReactInstance()) {
      return;
    }
    reactContext.runOnJSQueueThread(() -> reactContext
      .getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter.class)
      .emit(MESSAGE_EVENT, null));
  }
}
