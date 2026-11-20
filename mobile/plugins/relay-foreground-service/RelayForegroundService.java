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
import java.util.Collections;
import java.util.Date;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;
import java.util.UUID;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;

public final class RelayForegroundService extends Service {
  private static final String CHANNEL_ID = "relay_connection";
  private static final int NOTIFICATION_ID = 18080;
  private static final String PREFS = "relay_foreground_service";
  private static final String KEY_ENABLED = "enabled";
  private static final String KEY_WEBSOCKET_URL = "websocket_url";
  private static final String KEY_USER_ID = "user_id";
  private static final String KEY_DEVICE_ID = "device_id";
  private static final String KEY_CLIENT_TOKEN = "client_token";
  private static final String KEY_PAIRING_ID = "pairing_id";
  private static final String KEY_AUTH_REJECTED = "auth_rejected";
  private static final int MAX_QUEUED_MESSAGES = 256;
  private static final int MAX_QUEUED_CHARS = 8 * 1024 * 1024;
  private static final long CONNECT_TIMEOUT_MS = 10000L;
  private static final long HEARTBEAT_TIMEOUT_MS = 10000L;
  private static final long HEARTBEAT_INTERVAL_MS = 25000L;
  private static final long MAX_RECONNECT_DELAY_MS = 30000L;
  private static final String STOPPED_STATUS = "Relay 后台服务已停止";
  private static final String CONNECTING_STATUS = "Relay 正在连接";
  private static final String REJECTED_STATUS = "Relay 配对已失效，请重新配对";

  // The Android main looper owns all transport state, including bridge commands,
  // service lifecycle, timers and socket callbacks. Intents only wake the service;
  // neither a delayed start intent nor a sticky restart can restore retired credentials.
  private static final Handler MAIN = new Handler(Looper.getMainLooper());
  private static boolean restored;
  private static Configuration requested;
  private static boolean authRejected;
  private static boolean startPending;
  private static RelayForegroundService instance;
  private static EventSink eventSink;
  private static Snapshot snapshot = new Snapshot("stopped", STOPPED_STATUS, "", "", "", 0);

  private OkHttpClient httpClient;
  private WebSocket.Factory socketFactory;
  private boolean foreground;
  private boolean stopping;
  private Configuration configured;
  private Connection active;
  private Runnable reconnect;
  private int reconnectAttempt;

  static final class Snapshot {
    final String state;
    final String status;
    final String error;
    final String pairingID;
    final String connectionID;
    final long revision;

    Snapshot(String state, String status, String error, String pairingID, String connectionID, long revision) {
      this.state = state;
      this.status = status;
      this.error = error;
      this.pairingID = pairingID;
      this.connectionID = connectionID;
      this.revision = revision;
    }
  }

  interface EventSink {
    void onState(Snapshot state);
    void onMessagesAvailable();
  }

  private static final class Configuration {
    final String websocketURL;
    final String userID;
    final String deviceID;
    final String clientToken;
    final String pairingID;
    final Request request;

    private Configuration(String websocketURL, String userID, String deviceID, String clientToken,
                          String pairingID, Request request) {
      this.websocketURL = websocketURL;
      this.userID = userID;
      this.deviceID = deviceID;
      this.clientToken = clientToken;
      this.pairingID = pairingID;
      this.request = request;
    }

    static Configuration parse(String websocketURL, String userID, String deviceID, String clientToken,
                               String pairingID) {
      String url = trim(websocketURL);
      String user = trim(userID);
      String device = trim(deviceID);
      String token = trim(clientToken);
      if (pairingID == null || pairingID.isEmpty() || !pairingID.equals(pairingID.trim())
          || user.isEmpty() || device.isEmpty() || token.isEmpty()
          || !(url.regionMatches(true, 0, "ws://", 0, 5) || url.regionMatches(true, 0, "wss://", 0, 6))) {
        return null;
      }
      try {
        Request request = new Request.Builder().url(url.replaceFirst("(?i)^ws", "http")).build();
        if (!request.url().encodedUsername().isEmpty() || !request.url().encodedPassword().isEmpty()
            || request.url().encodedQuery() != null || request.url().encodedFragment() != null) {
          return null;
        }
        return new Configuration(url, user, device, token, pairingID, request);
      } catch (IllegalArgumentException ignored) {
        return null;
      }
    }

    boolean matches(Configuration other) {
      return other != null && pairingID.equals(other.pairingID) && request.url().equals(other.request.url())
        && userID.equals(other.userID) && deviceID.equals(other.deviceID) && clientToken.equals(other.clientToken);
    }
  }

  // The object identity is the internal generation. Its public UUID belongs to
  // exactly one WebSocket, and no callback ever reads another socket's identity.
  private static final class Connection {
    final Configuration config;
    final String id = UUID.randomUUID().toString();
    final ArrayDeque<String> messages = new ArrayDeque<>();
    WebSocket socket;
    volatile boolean retired;
    boolean authenticated;
    String heartbeatID = "";
    Runnable deadline;
    Runnable heartbeat;
    int bufferedChars;

    Connection(Configuration config) {
      this.config = config;
    }

    void clearMessages() {
      messages.clear();
      bufferedChars = 0;
    }

    boolean offer(String message) {
      while (messages.size() >= MAX_QUEUED_MESSAGES || bufferedChars + message.length() > MAX_QUEUED_CHARS) {
        if (!authenticated || messages.isEmpty()) {
          return false;
        }
        bufferedChars -= messages.removeFirst().length();
      }
      messages.addLast(message);
      bufferedChars += message.length();
      return true;
    }
  }

  @Override
  public void onCreate() {
    super.onCreate();
    instance = this;
    httpClient = new OkHttpClient.Builder()
      .connectTimeout(CONNECT_TIMEOUT_MS, TimeUnit.MILLISECONDS)
      .readTimeout(0, TimeUnit.MILLISECONDS)
      .pingInterval(20, TimeUnit.SECONDS)
      .retryOnConnectionFailure(false)
      .followRedirects(false)
      .followSslRedirects(false)
      .build();
    socketFactory = httpClient;
    createNotificationChannel();
  }

  // Deterministic tests can supply fake sockets before starting the service.
  void setSocketFactory(WebSocket.Factory factory) {
    socketFactory = factory;
  }

  @Override
  public int onStartCommand(@Nullable Intent intent, int flags, int startId) {
    startPending = false;
    restoreConfiguration(this);
    if (!foreground) {
      // Even if a stop raced this wake-up, satisfy the foreground-service start
      // requirement before stopping. The retired config is never used.
      try {
        Notification notification = buildNotification();
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
          startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_REMOTE_MESSAGING);
        } else {
          startForeground(NOTIFICATION_ID, notification);
        }
        foreground = true;
      } catch (RuntimeException ignored) {
        stopTransport();
        if (requested != null) {
          publish("error", "无法启动 Relay 后台服务", "请在应用前台重试", "");
        }
        return START_NOT_STICKY;
      }
    }
    stopping = false;
    if (requested == null) {
      stopTransport();
      return START_NOT_STICKY;
    }
    configure(false);
    return START_STICKY;
  }

  @Override
  public void onDestroy() {
    cancelReconnect();
    retireSocket();
    configured = null;
    if (httpClient != null) {
      httpClient.dispatcher().cancelAll();
      httpClient.connectionPool().evictAll();
    }
    if (instance == this) {
      instance = null;
      if (!stopping && !startPending) {
        publishIdle();
      }
    }
    if (foreground) {
      if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
        stopForeground(STOP_FOREGROUND_REMOVE);
      } else {
        stopForeground(true);
      }
      foreground = false;
    }
    super.onDestroy();
  }

  @Nullable
  @Override
  public IBinder onBind(Intent intent) {
    return null;
  }

  static void onMain(Runnable task) {
    if (Looper.myLooper() == MAIN.getLooper()) {
      task.run();
    } else {
      MAIN.post(task);
    }
  }

  static void setEventSink(EventSink sink) {
    onMain(() -> {
      eventSink = sink;
      sink.onState(snapshot);
      Connection connection = instance == null ? null : instance.active;
      if (connection != null && connection.authenticated && !connection.messages.isEmpty()) {
        sink.onMessagesAvailable();
      }
    });
  }

  static void clearEventSink(EventSink sink) {
    onMain(() -> {
      if (eventSink == sink) {
        eventSink = null;
      }
    });
  }

  // The scoped bridge commands below are called on MAIN. They resolve only after
  // applying the request to the authoritative config or authenticated socket.
  static boolean startScoped(Context context, String websocketURL, String userID, String deviceID,
                             String clientToken, String pairingID, boolean retry) {
    Configuration next = Configuration.parse(websocketURL, userID, deviceID, clientToken, pairingID);
    if (next == null) {
      return false;
    }
    restoreConfiguration(context);
    SharedPreferences preferences = preferences(context);
    boolean changed = !next.matches(requested);
    if (changed) {
      if (instance != null) {
        instance.cancelReconnect();
        instance.retireSocket();
      }
      requested = next;
      authRejected = false;
      // Commit before acknowledging a new pairing so a sticky restart cannot
      // race the write and use the replaced pairing.
      if (!preferences.edit().clear()
          .putBoolean(KEY_ENABLED, true)
          .putString(KEY_PAIRING_ID, next.pairingID)
          .putString(KEY_WEBSOCKET_URL, next.websocketURL)
          .putString(KEY_USER_ID, next.userID)
          .putString(KEY_DEVICE_ID, next.deviceID)
          .putString(KEY_CLIENT_TOKEN, next.clientToken)
          .commit()) {
        requested = null;
        preferences.edit().clear().commit();
        if (instance != null) {
          instance.stopTransport();
        }
        publish("error", "无法保存 Relay 配对", "请在应用前台重试", "");
        return false;
      }
    } else if (authRejected && retry) {
      if (!preferences.edit().remove(KEY_AUTH_REJECTED).commit()) {
        preferences.edit().putBoolean(KEY_AUTH_REJECTED, true).apply();
        return false;
      }
      authRejected = false;
    }

    if (instance != null && instance.foreground && !instance.stopping) {
      instance.configure(retry);
    } else if (!authRejected) {
      if (!startPending) {
        try {
          Intent wake = new Intent(context, RelayForegroundService.class);
          if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            if (context.startForegroundService(wake) == null) {
              return foregroundStartFailed();
            }
          } else if (context.startService(wake) == null) {
            return foregroundStartFailed();
          }
          startPending = true;
        } catch (RuntimeException ignored) {
          return foregroundStartFailed();
        }
      }
      publish("connecting", CONNECTING_STATUS, "", "");
    }
    return true;
  }

  private static boolean foregroundStartFailed() {
    publish("error", "无法启动 Relay 后台服务", "请在应用前台重试", "");
    return false;
  }

  static boolean stopScoped(Context context, String pairingID) {
    if (pairingID == null) {
      return false;
    }
    restoreConfiguration(context);
    if (!pairingID.isEmpty() && (requested == null || !requested.pairingID.equals(pairingID))) {
      return false;
    }
    requested = null;
    authRejected = false;
    if (instance != null && instance.foreground) {
      instance.stopTransport();
    }
    boolean cleared = preferences(context).edit().clear().commit();
    publishIdle();
    return cleared;
  }

  static boolean sendScoped(String pairingID, String connectionID, String message) {
    RelayForegroundService current = instance;
    Connection connection = scopedConnection(pairingID, connectionID);
    if (current == null || connection == null || message == null || message.isEmpty()) {
      return false;
    }
    if (offerToSocket(connection.socket, message)) {
      return true;
    }
    current.socketLost(connection, "Relay 消息发送失败");
    return false;
  }

  static Snapshot getScopedState(Context context) {
    restoreConfiguration(context);
    return snapshot;
  }

  static List<String> drainScopedMessages(String pairingID, String connectionID) {
    Connection connection = scopedConnection(pairingID, connectionID);
    if (connection == null || connection.messages.isEmpty()) {
      return Collections.emptyList();
    }
    List<String> drained = new ArrayList<>(connection.messages);
    connection.clearMessages();
    return drained;
  }

  @Nullable
  private static Connection scopedConnection(String pairingID, String connectionID) {
    Connection connection = instance == null ? null : instance.active;
    return connection != null && connection.authenticated && connection.config == requested
      && connection.config.pairingID.equals(pairingID) && connection.id.equals(connectionID)
      ? connection : null;
  }

  private static SharedPreferences preferences(Context context) {
    return context.getSharedPreferences(PREFS, MODE_PRIVATE);
  }

  private static void restoreConfiguration(Context context) {
    if (restored) {
      return;
    }
    SharedPreferences preferences = preferences(context);
    boolean rejected = false;
    try {
      if (preferences.getBoolean(KEY_ENABLED, false)) {
        requested = Configuration.parse(preferences.getString(KEY_WEBSOCKET_URL, ""),
          preferences.getString(KEY_USER_ID, ""), preferences.getString(KEY_DEVICE_ID, ""),
          preferences.getString(KEY_CLIENT_TOKEN, ""), preferences.getString(KEY_PAIRING_ID, ""));
        rejected = requested != null && preferences.getBoolean(KEY_AUTH_REJECTED, false);
      }
    } catch (ClassCastException ignored) {
      requested = null;
    }
    if (requested == null && !preferences.getAll().isEmpty()) {
      // The JS side must migrate and explicitly start with a pairing ID. Do not
      // connect using or retain the old unscoped values in native preferences.
      preferences.edit().clear().commit();
    }
    authRejected = rejected;
    restored = true;
    publishIdle();
  }

  private void configure(boolean retry) {
    if (configured != requested) {
      configured = requested;
      // This is a different configuration, not a successful reconnect attempt.
      reconnectAttempt = 0;
    }
    if (authRejected) {
      publishIdle();
      return;
    }
    if (retry) {
      cancelReconnect();
    }
    if (reconnect == null) {
      connectIfNeeded();
    }
  }

  private void connectIfNeeded() {
    if (!foreground || stopping || configured == null || configured != requested || authRejected || active != null) {
      return;
    }
    Connection connection = new Connection(configured);
    active = connection;
    try {
      connection.socket = socketFactory.newWebSocket(connection.config.request, new RelaySocketListener(connection));
    } catch (RuntimeException ignored) {
      socketLost(connection, "Relay 无法建立连接");
      return;
    }
    publish("connecting", CONNECTING_STATUS, "", connection.id);
    setDeadline(connection, CONNECT_TIMEOUT_MS, () -> socketLost(connection, "Relay 连接超时"));
  }

  private boolean isCurrent(Connection connection, WebSocket socket) {
    return active == connection && connection.config == requested && connection.socket == socket;
  }

  private void socketOpened(Connection connection, WebSocket socket) {
    if (!isCurrent(connection, socket)) {
      return;
    }
    publish("authenticating", "Relay 正在验证配对", "", connection.id);
    sendHeartbeat(connection);
  }

  private void sendHeartbeat(Connection connection) {
    if (!isCurrent(connection, connection.socket) || !connection.heartbeatID.isEmpty()) {
      return;
    }
    Configuration config = connection.config;
    String id = UUID.randomUUID().toString();
    try {
      JSONObject message = new JSONObject()
        .put("type", "heartbeat")
        .put("request_id", id)
        .put("user_id", config.userID)
        .put("device_id", config.deviceID)
        .put("client_token", config.clientToken)
        .put("payload", new JSONObject().put("time", isoNow()));
      connection.heartbeatID = id;
      if (!offerToSocket(connection.socket, message.toString())) {
        socketLost(connection, "Relay 心跳发送失败");
        return;
      }
      setDeadline(connection, HEARTBEAT_TIMEOUT_MS, () -> {
        if (id.equals(connection.heartbeatID)) {
          socketLost(connection, "Relay 认证或心跳回执超时");
        }
      });
    } catch (JSONException ignored) {
      socketLost(connection, "Relay 心跳构造失败");
    }
  }

  private static boolean offerToSocket(WebSocket socket, String message) {
    try {
      return socket != null && socket.send(message);
    } catch (RuntimeException ignored) {
      return false;
    }
  }

  private void socketMessage(Connection connection, WebSocket socket, String text) {
    if (!isCurrent(connection, socket)) {
      return;
    }
    JSONObject message;
    try {
      message = new JSONObject(text);
    } catch (JSONException ignored) {
      return;
    }
    Object type = message.opt("type");
    Object userID = message.opt("user_id");
    Object deviceID = message.opt("device_id");
    if (!(type instanceof String) || !matchesIdentity(userID, connection.config.userID)
        || !matchesIdentity(deviceID, connection.config.deviceID)) {
      return;
    }

    boolean heartbeat = "heartbeat".equals(type);
    boolean correlated = !connection.heartbeatID.isEmpty() && connection.heartbeatID.equals(message.opt("request_id"));
    if (heartbeat || (correlated && "error".equals(type))) {
      if (correlated && connection.config.userID.equals(userID) && connection.config.deviceID.equals(deviceID)) {
        JSONObject payload = message.optJSONObject("payload");
        if (payload != null) {
          if (heartbeat && "heartbeat".equals(payload.opt("received"))) {
            heartbeatAcknowledged(connection);
          } else if (!heartbeat && "client_auth_rejected".equals(payload.opt("error_code"))) {
            authRejected = true;
            retireSocket();
            preferences(this).edit().putBoolean(KEY_AUTH_REJECTED, true).commit();
            publishIdle();
          } else if (!heartbeat && ("client_auth_unavailable".equals(payload.opt("error_code"))
              || "client token is invalid or expired".equals(payload.opt("message")))) {
            socketLost(connection, "Relay 暂时无法验证配对");
          }
        }
      }
      return;
    }

    // Replay frames may arrive before the auth ACK. They share the socket's
    // buffer but are not observable or drainable until that socket authenticates.
    if (message.remove("client_token") != null) {
      text = message.toString();
    }
    boolean wasEmpty = connection.messages.isEmpty();
    if (!connection.offer(text)) {
      socketLost(connection, "Relay 待恢复消息过多");
      return;
    }
    if (wasEmpty && connection.authenticated) {
      emitMessagesAvailable();
    }
  }

  private static boolean matchesIdentity(Object value, String expected) {
    return value == null || value == JSONObject.NULL || "".equals(value) || expected.equals(value);
  }

  private void heartbeatAcknowledged(Connection connection) {
    clearDeadline(connection);
    connection.heartbeatID = "";
    reconnectAttempt = 0;
    connection.heartbeat = () -> sendHeartbeat(connection);
    MAIN.postDelayed(connection.heartbeat, HEARTBEAT_INTERVAL_MS);
    if (!connection.authenticated) {
      connection.authenticated = true;
      publish("connected", "Relay 已连接", "", connection.id);
      if (!connection.messages.isEmpty()) {
        emitMessagesAvailable();
      }
    }
  }

  private void setDeadline(Connection connection, long delay, Runnable action) {
    clearDeadline(connection);
    connection.deadline = action;
    MAIN.postDelayed(action, delay);
  }

  private static void clearDeadline(Connection connection) {
    if (connection.deadline != null) {
      MAIN.removeCallbacks(connection.deadline);
      connection.deadline = null;
    }
  }

  private void socketLost(Connection connection, String reason) {
    if (active != connection || requested != connection.config) {
      return;
    }
    retireSocket();
    publish("reconnecting", "Relay 正在重连", reason, "");
    long delay = Math.min(MAX_RECONNECT_DELAY_MS, 500L << reconnectAttempt);
    reconnectAttempt = Math.min(reconnectAttempt + 1, 6);
    reconnect = () -> {
      reconnect = null;
      connectIfNeeded();
    };
    MAIN.postDelayed(reconnect, delay);
  }

  private void cancelReconnect() {
    if (reconnect != null) {
      MAIN.removeCallbacks(reconnect);
      reconnect = null;
    }
  }

  private void retireSocket() {
    Connection connection = active;
    active = null;
    if (connection != null) {
      connection.retired = true;
      clearDeadline(connection);
      if (connection.heartbeat != null) {
        MAIN.removeCallbacks(connection.heartbeat);
      }
      connection.clearMessages();
      if (connection.socket != null) {
        connection.socket.cancel();
      }
    }
  }

  private void stopTransport() {
    stopping = true;
    cancelReconnect();
    retireSocket();
    configured = null;
    stopSelf();
  }

  private static void publishIdle() {
    if (authRejected) {
      publish("auth_rejected", REJECTED_STATUS, REJECTED_STATUS, "");
    } else {
      publish("stopped", STOPPED_STATUS, "", "");
    }
  }

  private static void publish(String state, String status, String error, String connectionID) {
    String pairingID = requested == null ? "" : requested.pairingID;
    if (state.equals(snapshot.state) && status.equals(snapshot.status) && error.equals(snapshot.error)
        && pairingID.equals(snapshot.pairingID) && connectionID.equals(snapshot.connectionID)) {
      return;
    }
    snapshot = new Snapshot(state, status, error, pairingID, connectionID, snapshot.revision + 1);
    if (instance != null && instance.foreground && !instance.stopping) {
      NotificationManager manager = instance.getSystemService(NotificationManager.class);
      if (manager != null) {
        manager.notify(NOTIFICATION_ID, instance.buildNotification());
      }
    }
    if (eventSink != null) {
      eventSink.onState(snapshot);
    }
  }

  private static void emitMessagesAvailable() {
    if (eventSink != null) {
      eventSink.onMessagesAvailable();
    }
  }

  private Notification buildNotification() {
    Intent launchIntent = new Intent(this, MainActivity.class);
    launchIntent.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
    PendingIntent pendingIntent = PendingIntent.getActivity(
      this, 0, launchIntent, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);

    return new NotificationCompat.Builder(this, CHANNEL_ID)
      .setSmallIcon(R.drawable.relay_notification)
      .setContentTitle(getString(R.string.app_name))
      .setContentText(snapshot.error.isEmpty() ? snapshot.status : snapshot.error)
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
    NotificationChannel channel = new NotificationChannel(CHANNEL_ID, "Relay 连接", NotificationManager.IMPORTANCE_LOW);
    channel.setDescription("保持手机与 Relay 的后台连接");
    channel.setShowBadge(false);
    NotificationManager manager = getSystemService(NotificationManager.class);
    if (manager != null) {
      manager.createNotificationChannel(channel);
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

  private final class RelaySocketListener extends WebSocketListener {
    private final Connection connection;
    // Bound the handoff to MAIN as well as the socket's own message buffer.
    private final AtomicInteger postedCount = new AtomicInteger();
    private final AtomicInteger postedChars = new AtomicInteger();
    private boolean overflowed; // Only accessed on the OkHttp reader thread.

    RelaySocketListener(Connection connection) {
      this.connection = connection;
    }

    @Override
    public void onOpen(WebSocket socket, Response response) {
      MAIN.post(() -> socketOpened(connection, socket));
    }

    @Override
    public void onMessage(WebSocket socket, String text) {
      if (connection.retired || overflowed) {
        return;
      }
      int count = postedCount.incrementAndGet();
      int chars = postedChars.addAndGet(text.length());
      if (count > MAX_QUEUED_MESSAGES || chars > MAX_QUEUED_CHARS) {
        overflowed = true;
        postedCount.decrementAndGet();
        postedChars.addAndGet(-text.length());
        postLoss(socket, "Relay 待恢复消息过多");
        return;
      }
      MAIN.post(() -> {
        try {
          socketMessage(connection, socket, text);
        } finally {
          postedCount.decrementAndGet();
          postedChars.addAndGet(-text.length());
        }
      });
    }

    @Override
    public void onClosing(WebSocket socket, int code, String reason) {
      postLoss(socket, "Relay 连接已断开");
    }

    @Override
    public void onClosed(WebSocket socket, int code, String reason) {
      postLoss(socket, "Relay 连接已断开");
    }

    @Override
    public void onFailure(WebSocket socket, Throwable throwable, @Nullable Response response) {
      postLoss(socket, "Relay 连接失败，请检查网络和服务器");
    }

    private void postLoss(WebSocket socket, String reason) {
      MAIN.post(() -> {
        if (isCurrent(connection, socket)) {
          socketLost(connection, reason);
        }
      });
    }
  }
}
