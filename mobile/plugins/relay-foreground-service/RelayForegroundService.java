package com.myai.mobile.relay;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ServiceInfo;
import android.net.ConnectivityManager;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;

import androidx.annotation.Nullable;
import androidx.core.app.NotificationCompat;

import com.myai.mobile.MainActivity;
import com.myai.mobile.R;

import org.json.JSONException;
import org.json.JSONObject;

import java.text.SimpleDateFormat;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;
import java.util.concurrent.TimeUnit;

import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;

public final class RelayForegroundService extends Service {
  public static final String ACTION_START = "com.myai.mobile.relay.START";
  public static final String ACTION_STOP = "com.myai.mobile.relay.STOP";
  public static final String EXTRA_WEBSOCKET_URL = "websocket_url";
  public static final String EXTRA_USER_ID = "user_id";
  public static final String EXTRA_DEVICE_ID = "device_id";
  public static final String EXTRA_CLIENT_TOKEN = "client_token";

  private static final String CHANNEL_ID = "relay_connection";
  private static final int NOTIFICATION_ID = 18080;
  private static final String PREFS = "relay_foreground_service";
  private static final String KEY_ENABLED = "enabled";
  private static final String KEY_WEBSOCKET_URL = "websocket_url";
  private static final String KEY_USER_ID = "user_id";
  private static final String KEY_DEVICE_ID = "device_id";
  private static final String KEY_CLIENT_TOKEN = "client_token";
  private static final String DEFAULT_STATUS = "Relay 后台服务运行中";
  private static final int MAX_QUEUED_MESSAGES = 256;
  private static final long HEARTBEAT_INTERVAL_MS = 25000L;
  private static final long MAX_RECONNECT_DELAY_MS = 30000L;

  private static final Object MESSAGE_LOCK = new Object();
  private static final ArrayDeque<String> MESSAGE_QUEUE = new ArrayDeque<>();
  private static volatile boolean running;
  private static volatile String state = "stopped";
  private static volatile String statusText = DEFAULT_STATUS;
  private static volatile String lastError = "";
  private static volatile RelayForegroundService instance;
  private static volatile EventSink eventSink;

  private final Object socketLock = new Object();
  private final Handler handler = new Handler(Looper.getMainLooper());
  private final Runnable heartbeatRunnable = new Runnable() {
    @Override
    public void run() {
      sendHeartbeat();
      synchronized (socketLock) {
        if (enabled && webSocket != null && "connected".equals(state)) {
          handler.postDelayed(this, HEARTBEAT_INTERVAL_MS);
        }
      }
    }
  };
  private final Runnable reconnectRunnable = new Runnable() {
    @Override
    public void run() {
      synchronized (socketLock) {
        reconnectScheduled = false;
      }
      connectIfNeeded();
    }
  };

  private OkHttpClient httpClient;
  private ConnectivityManager connectivityManager;
  private ConnectivityManager.NetworkCallback networkCallback;
  private WebSocket webSocket;
  private String websocketURL = "";
  private String userID = "";
  private String deviceID = "";
  private String clientToken = "";
  private boolean enabled;
  private boolean reconnectScheduled;
  private int reconnectAttempt;
  private long socketGeneration;

  public interface EventSink {
    void onState(String state, String status, String error);
    void onMessagesAvailable();
  }

  @Override
  public void onCreate() {
    super.onCreate();
    instance = this;
    loadPersistedConfiguration();
    httpClient = new OkHttpClient.Builder()
      .connectTimeout(10, TimeUnit.SECONDS)
      .readTimeout(0, TimeUnit.MILLISECONDS)
      .pingInterval(20, TimeUnit.SECONDS)
      .retryOnConnectionFailure(true)
      .build();
    registerNetworkCallback();
    createNotificationChannel();
  }

  @Override
  public int onStartCommand(@Nullable Intent intent, int flags, int startId) {
    if (intent != null && ACTION_STOP.equals(intent.getAction())) {
      disableAndStop();
      return START_NOT_STICKY;
    }

    Notification notification = buildNotification();
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
      startForeground(
        NOTIFICATION_ID,
        notification,
        ServiceInfo.FOREGROUND_SERVICE_TYPE_REMOTE_MESSAGING
      );
    } else {
      startForeground(NOTIFICATION_ID, notification);
    }
    running = true;

    if (intent != null && ACTION_START.equals(intent.getAction())) {
      configureFromIntent(intent);
    } else {
      loadPersistedConfiguration();
    }
    if (hasConfiguration()) {
      connectIfNeeded();
    } else {
      publishState("error", "等待 Relay 配置", "Relay URL、用户或 Token 为空");
    }
    return START_STICKY;
  }

  @Override
  public void onDestroy() {
    handler.removeCallbacks(heartbeatRunnable);
    handler.removeCallbacks(reconnectRunnable);
    unregisterNetworkCallback();
    synchronized (socketLock) {
      enabled = false;
      cancelSocketLocked();
    }
    if (httpClient != null) {
      httpClient.dispatcher().cancelAll();
      httpClient.connectionPool().evictAll();
    }
    running = false;
    state = "stopped";
    statusText = DEFAULT_STATUS;
    instance = null;
    publishState("stopped", "Relay 后台服务已停止", "");
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
      stopForeground(STOP_FOREGROUND_REMOVE);
    } else {
      stopForeground(true);
    }
    super.onDestroy();
  }

  @Nullable
  @Override
  public IBinder onBind(Intent intent) {
    return null;
  }

  public static boolean isRunning() {
    return running;
  }

  public static String getState() {
    return state;
  }

  public static String getStatusText() {
    return statusText;
  }

  public static String getLastError() {
    return lastError;
  }

  public static void setEventSink(EventSink sink) {
    eventSink = sink;
    if (sink != null) {
      sink.onState(state, statusText, lastError);
      synchronized (MESSAGE_LOCK) {
        if (!MESSAGE_QUEUE.isEmpty()) {
          sink.onMessagesAvailable();
        }
      }
    }
  }

  public static List<String> drainMessages() {
    List<String> messages = new ArrayList<>();
    synchronized (MESSAGE_LOCK) {
      while (!MESSAGE_QUEUE.isEmpty()) {
        messages.add(MESSAGE_QUEUE.removeFirst());
      }
    }
    return messages;
  }

  public static boolean sendMessage(String message) {
    RelayForegroundService current = instance;
    if (current == null) {
      return false;
    }
    WebSocket socket;
    synchronized (current.socketLock) {
      socket = current.webSocket;
      if (!current.enabled || socket == null || !"connected".equals(state)) {
        return false;
      }
    }
    boolean sent = socket.send(message);
    if (!sent) {
      current.handleSocketLost(current.socketGeneration, "消息发送失败");
    }
    return sent;
  }

  public static void updateStatus(Context context, String status) {
    String normalizedStatus = status == null ? "" : status.trim();
    statusText = normalizedStatus.isEmpty() ? DEFAULT_STATUS : normalizedStatus;
    RelayForegroundService current = instance;
    if (current != null && running) {
      current.updateNotification();
      current.emitState();
    }
  }

  public static void requestStop(Context context) {
    Context applicationContext = context.getApplicationContext();
    applicationContext.getSharedPreferences(PREFS, MODE_PRIVATE)
      .edit()
      .putBoolean(KEY_ENABLED, false)
      .apply();
    RelayForegroundService current = instance;
    if (current != null) {
      current.disableAndStop();
    } else {
      applicationContext.stopService(new Intent(applicationContext, RelayForegroundService.class));
    }
  }

  private void configureFromIntent(Intent intent) {
    String nextURL = trim(intent.getStringExtra(EXTRA_WEBSOCKET_URL));
    String nextUserID = trim(intent.getStringExtra(EXTRA_USER_ID));
    String nextDeviceID = trim(intent.getStringExtra(EXTRA_DEVICE_ID));
    String nextToken = trim(intent.getStringExtra(EXTRA_CLIENT_TOKEN));
    synchronized (socketLock) {
      boolean changed = !websocketURL.equals(nextURL)
        || !userID.equals(nextUserID)
        || !deviceID.equals(nextDeviceID)
        || !clientToken.equals(nextToken);
      websocketURL = nextURL;
      userID = nextUserID;
      deviceID = nextDeviceID;
      clientToken = nextToken;
      // Compute validity from the new values themselves. Using hasConfiguration
      // here would include the old enabled flag (false on the first start),
      // preventing a newly configured service from ever connecting.
      enabled = hasConnectionParameters();
      reconnectAttempt = 0;
      if (changed) {
        cancelSocketLocked();
      }
    }
    persistConfiguration();
  }

  private void loadPersistedConfiguration() {
    SharedPreferences preferences = getSharedPreferences(PREFS, MODE_PRIVATE);
    websocketURL = trim(preferences.getString(KEY_WEBSOCKET_URL, ""));
    userID = trim(preferences.getString(KEY_USER_ID, ""));
    deviceID = trim(preferences.getString(KEY_DEVICE_ID, ""));
    clientToken = trim(preferences.getString(KEY_CLIENT_TOKEN, ""));
    enabled = preferences.getBoolean(KEY_ENABLED, false);
  }

  private void persistConfiguration() {
    getSharedPreferences(PREFS, MODE_PRIVATE)
      .edit()
      .putBoolean(KEY_ENABLED, enabled)
      .putString(KEY_WEBSOCKET_URL, websocketURL)
      .putString(KEY_USER_ID, userID)
      .putString(KEY_DEVICE_ID, deviceID)
      .putString(KEY_CLIENT_TOKEN, clientToken)
      .apply();
  }

  private boolean hasConfiguration() {
    return enabled && hasConnectionParameters();
  }

  private boolean hasConnectionParameters() {
    return !websocketURL.isEmpty() && !userID.isEmpty() && !deviceID.isEmpty() && !clientToken.isEmpty();
  }

  private void connectIfNeeded() {
    final long generation;
    synchronized (socketLock) {
      if (!hasConfiguration() || webSocket != null) {
        return;
      }
      reconnectScheduled = false;
      generation = ++socketGeneration;
      publishState("connecting", "Relay 正在连接", "");
      Request request = new Request.Builder().url(websocketURL).build();
      webSocket = httpClient.newWebSocket(request, new RelayWebSocketListener(generation));
    }
  }

  private void sendHeartbeat() {
    WebSocket socket;
    synchronized (socketLock) {
      socket = webSocket;
      if (!enabled || socket == null || !"connected".equals(state)) {
        return;
      }
    }
    try {
      JSONObject message = new JSONObject();
      message.put("type", "heartbeat");
      message.put("user_id", userID);
      message.put("device_id", deviceID);
      message.put("client_token", clientToken);
      JSONObject payload = new JSONObject();
      payload.put("time", isoNow());
      message.put("payload", payload);
      if (!socket.send(message.toString())) {
        handleSocketLost(socketGeneration, "Relay 心跳发送失败");
      }
    } catch (JSONException error) {
      handleSocketLost(socketGeneration, "Relay 心跳构造失败");
    }
  }

  private void handleSocketOpened(long generation, WebSocket socket) {
    synchronized (socketLock) {
      if (generation != socketGeneration || socket != webSocket || !enabled) {
        socket.cancel();
        return;
      }
      reconnectAttempt = 0;
      reconnectScheduled = false;
      state = "connected";
    }
    publishState("connected", "Relay 已连接", "");
    handler.removeCallbacks(heartbeatRunnable);
    sendHeartbeat();
    handler.postDelayed(heartbeatRunnable, HEARTBEAT_INTERVAL_MS);
  }

  private void handleSocketMessage(long generation, WebSocket socket, String text) {
    synchronized (socketLock) {
      if (generation != socketGeneration || socket != webSocket) {
        return;
      }
    }
    if (isHeartbeat(text)) {
      return;
    }
    synchronized (MESSAGE_LOCK) {
      if (MESSAGE_QUEUE.size() >= MAX_QUEUED_MESSAGES) {
        MESSAGE_QUEUE.removeFirst();
      }
      MESSAGE_QUEUE.addLast(text);
    }
    EventSink sink = eventSink;
    if (sink != null) {
      sink.onMessagesAvailable();
    }
  }

  private void handleSocketLost(long generation, String error) {
    synchronized (socketLock) {
      if (generation != socketGeneration || webSocket == null) {
        return;
      }
      cancelSocketLocked();
      if (!hasConfiguration()) {
        publishState("stopped", "Relay 已停止", "");
        return;
      }
      publishState("reconnecting", "Relay 正在重连", error == null ? "" : error);
      scheduleReconnectLocked();
    }
  }

  private void scheduleReconnectLocked() {
    if (reconnectScheduled || !hasConfiguration()) {
      return;
    }
    long delay = Math.min(MAX_RECONNECT_DELAY_MS, 500L * (1L << Math.min(reconnectAttempt, 6)));
    reconnectAttempt = Math.min(reconnectAttempt + 1, 6);
    reconnectScheduled = true;
    handler.removeCallbacks(reconnectRunnable);
    handler.postDelayed(reconnectRunnable, delay);
  }

  private void cancelSocketLocked() {
    handler.removeCallbacks(heartbeatRunnable);
    WebSocket oldSocket = webSocket;
    webSocket = null;
    if (oldSocket != null) {
      oldSocket.cancel();
    }
  }

  private void disableAndStop() {
    synchronized (socketLock) {
      enabled = false;
      reconnectScheduled = false;
      handler.removeCallbacks(reconnectRunnable);
      cancelSocketLocked();
    }
    persistConfiguration();
    stopSelf();
  }

  private void registerNetworkCallback() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.N) {
      return;
    }
    connectivityManager = (ConnectivityManager) getSystemService(CONNECTIVITY_SERVICE);
    if (connectivityManager == null) {
      return;
    }
    networkCallback = new ConnectivityManager.NetworkCallback() {
      @Override
      public void onAvailable(android.net.Network network) {
        handler.post(() -> {
          synchronized (socketLock) {
            if (enabled && webSocket == null) {
              reconnectScheduled = false;
              reconnectAttempt = 0;
            }
          }
          connectIfNeeded();
        });
      }
    };
    try {
      connectivityManager.registerDefaultNetworkCallback(networkCallback);
    } catch (RuntimeException error) {
      networkCallback = null;
    }
  }

  private void unregisterNetworkCallback() {
    if (connectivityManager == null || networkCallback == null || Build.VERSION.SDK_INT < Build.VERSION_CODES.N) {
      return;
    }
    try {
      connectivityManager.unregisterNetworkCallback(networkCallback);
    } catch (RuntimeException ignored) {
      // The callback may already have been removed by the system.
    }
    networkCallback = null;
  }

  private void publishState(String nextState, String nextStatus, String error) {
    state = nextState;
    statusText = nextStatus == null || nextStatus.trim().isEmpty() ? DEFAULT_STATUS : nextStatus;
    lastError = error == null ? "" : error;
    updateNotification();
    emitState();
  }

  private void emitState() {
    EventSink sink = eventSink;
    if (sink != null) {
      sink.onState(state, statusText, lastError);
    }
  }

  private void updateNotification() {
    if (!running) {
      return;
    }
    NotificationManager manager = getSystemService(NotificationManager.class);
    if (manager != null) {
      manager.notify(NOTIFICATION_ID, buildNotification());
    }
  }

  private Notification buildNotification() {
    Intent launchIntent = new Intent(this, MainActivity.class);
    launchIntent.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
    PendingIntent pendingIntent = PendingIntent.getActivity(
      this,
      0,
      launchIntent,
      PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
    );

    return new NotificationCompat.Builder(this, CHANNEL_ID)
      .setSmallIcon(R.drawable.relay_notification)
      .setContentTitle(getString(R.string.app_name))
      .setContentText(statusText)
      .setContentIntent(pendingIntent)
      .setCategory(NotificationCompat.CATEGORY_SERVICE)
      .setPriority(NotificationCompat.PRIORITY_LOW)
      .setOngoing(true)
      .setShowWhen(false)
      .build();
  }

  private void createNotificationChannel() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
      return;
    }
    NotificationChannel channel = new NotificationChannel(
      CHANNEL_ID,
      "Relay 连接",
      NotificationManager.IMPORTANCE_LOW
    );
    channel.setDescription("保持手机与 Relay 的后台连接");
    channel.setShowBadge(false);
    NotificationManager manager = getSystemService(NotificationManager.class);
    if (manager != null) {
      manager.createNotificationChannel(channel);
    }
  }

  private static boolean isHeartbeat(String text) {
    try {
      return "heartbeat".equals(new JSONObject(text).optString("type"));
    } catch (JSONException ignored) {
      return false;
    }
  }

  private static String isoNow() {
    SimpleDateFormat format = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'", Locale.US);
    format.setTimeZone(TimeZone.getTimeZone("UTC"));
    return format.format(new Date());
  }

  private static String trim(String value) {
    return value == null ? "" : value.trim();
  }

  private final class RelayWebSocketListener extends WebSocketListener {
    private final long generation;

    private RelayWebSocketListener(long generation) {
      this.generation = generation;
    }

    @Override
    public void onOpen(WebSocket webSocket, Response response) {
      handleSocketOpened(generation, webSocket);
    }

    @Override
    public void onMessage(WebSocket webSocket, String text) {
      handleSocketMessage(generation, webSocket, text);
    }

    @Override
    public void onClosing(WebSocket webSocket, int code, String reason) {
      webSocket.close(code, reason);
    }

    @Override
    public void onClosed(WebSocket webSocket, int code, String reason) {
      handleSocketLost(generation, reason);
    }

    @Override
    public void onFailure(WebSocket webSocket, Throwable throwable, @Nullable Response response) {
      handleSocketLost(generation, throwable == null ? "Relay 连接失败" : throwable.getMessage());
    }
  }
}
