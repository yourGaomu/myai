package com.myai.mobile.relay;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;

import androidx.annotation.Nullable;
import androidx.core.app.NotificationCompat;

import com.myai.mobile.MainActivity;
import com.myai.mobile.R;

public final class RelayForegroundService extends Service {
  public static final String ACTION_START = "com.myai.mobile.relay.START";
  public static final String ACTION_STOP = "com.myai.mobile.relay.STOP";

  private static final String CHANNEL_ID = "relay_connection";
  private static final int NOTIFICATION_ID = 18080;
  private static final String DEFAULT_STATUS = "Relay 后台服务运行中";
  private static volatile boolean running;
  private static volatile String statusText = DEFAULT_STATUS;
  private static volatile RelayForegroundService instance;

  @Override
  public void onCreate() {
    super.onCreate();
    instance = this;
    createNotificationChannel();
  }

  @Override
  public int onStartCommand(@Nullable Intent intent, int flags, int startId) {
    if (intent != null && ACTION_STOP.equals(intent.getAction())) {
      stopSelf();
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
    return START_STICKY;
  }

  @Override
  public void onDestroy() {
    running = false;
    instance = null;
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

  public static void updateStatus(android.content.Context context, String status) {
    String normalizedStatus = status == null ? "" : status.trim();
    statusText = normalizedStatus.isEmpty() ? DEFAULT_STATUS : normalizedStatus;
    RelayForegroundService current = instance;
    if (current != null && running) {
      current.updateNotification();
    }
  }

  private void updateNotification() {
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
}
