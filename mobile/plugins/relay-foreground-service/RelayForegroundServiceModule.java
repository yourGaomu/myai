package com.myai.mobile.relay;

import com.facebook.react.bridge.Arguments;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.WritableArray;
import com.facebook.react.bridge.WritableMap;

public final class RelayForegroundServiceModule extends ReactContextBaseJavaModule {
  public static final String STATE_EVENT = "RelayForegroundServiceState";
  public static final String MESSAGE_EVENT = "RelayForegroundServiceMessage";

  private final ReactApplicationContext reactContext;
  private final RelayForegroundService.EventSink eventSink = new RelayForegroundService.EventSink() {
    @Override
    public void onState(RelayForegroundService.Snapshot state) {
      if (reactContext.hasActiveReactInstance()) {
        reactContext.emitDeviceEvent(STATE_EVENT, statePayload(state));
      }
    }

    @Override
    public void onMessagesAvailable() {
      if (reactContext.hasActiveReactInstance()) {
        reactContext.emitDeviceEvent(MESSAGE_EVENT, null);
      }
    }
  };

  public RelayForegroundServiceModule(ReactApplicationContext reactContext) {
    super(reactContext);
    this.reactContext = reactContext;
  }

  @Override
  public void initialize() {
    super.initialize();
    RelayForegroundService.setEventSink(eventSink);
  }

  @Override
  public void invalidate() {
    RelayForegroundService.clearEventSink(eventSink);
    super.invalidate();
  }

  @Override
  public String getName() {
    return "RelayForegroundService";
  }

  @ReactMethod
  public void startScoped(String websocketURL, String userID, String deviceID, String clientToken,
                          String pairingID, boolean retry, Promise promise) {
    RelayForegroundService.onMain(() -> promise.resolve(RelayForegroundService.startScoped(
      reactContext, websocketURL, userID, deviceID, clientToken, pairingID, retry)));
  }

  @ReactMethod
  public void stopScoped(String pairingID, Promise promise) {
    RelayForegroundService.onMain(() -> promise.resolve(RelayForegroundService.stopScoped(reactContext, pairingID)));
  }

  @ReactMethod
  public void sendScoped(String pairingID, String connectionID, String message, Promise promise) {
    RelayForegroundService.onMain(() -> promise.resolve(RelayForegroundService.sendScoped(pairingID, connectionID, message)));
  }

  @ReactMethod
  public void getScopedState(Promise promise) {
    RelayForegroundService.onMain(() -> promise.resolve(statePayload(RelayForegroundService.getScopedState(reactContext))));
  }

  @ReactMethod
  public void drainScopedMessages(String pairingID, String connectionID, Promise promise) {
    RelayForegroundService.onMain(() -> {
      WritableArray result = Arguments.createArray();
      for (String message : RelayForegroundService.drainScopedMessages(pairingID, connectionID)) {
        result.pushString(message);
      }
      promise.resolve(result);
    });
  }

  // React Native requires these methods when a native module emits events.
  @ReactMethod
  public void addListener(String eventName) {
    // no-op
  }

  @ReactMethod
  public void removeListeners(double count) {
    // no-op
  }

  private static WritableMap statePayload(RelayForegroundService.Snapshot state) {
    WritableMap payload = Arguments.createMap();
    payload.putString("state", state.state);
    payload.putString("status", state.status);
    if (!state.error.isEmpty()) {
      payload.putString("error", state.error);
    }
    payload.putString("pairingID", state.pairingID);
    payload.putString("connectionID", state.connectionID);
    payload.putDouble("revision", state.revision);
    return payload;
  }
}
