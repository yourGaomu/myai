import { useRef, useEffect } from "react";
import {
  Animated,
  Dimensions,
  Image,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";

import type { SessionSummary } from "../../protocol";
import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";

type Props = {
  activeSessionID?: string;
  buttonFeedback: ButtonFeedback;
  connected: boolean;
  deviceID: string;
  onClose: () => void;
  onNewSession?: () => void;
  onOpenSettings: () => void;
  onSelectSession?: (sessionID: string) => void;
  onSelectViewMode: (mode: ViewMode) => void;
  sessions: SessionSummary[];
  userID: string;
  viewMode: ViewMode;
  visible: boolean;
};

const SCREEN_WIDTH = Dimensions.get("window").width;
const DRAWER_WIDTH = Math.min(SCREEN_WIDTH * 0.82, 320);

export function SideDrawer({
  activeSessionID,
  buttonFeedback,
  connected,
  deviceID,
  onClose,
  onNewSession,
  onOpenSettings,
  onSelectSession,
  onSelectViewMode,
  sessions,
  userID,
  viewMode,
  visible,
}: Props) {
  const slideAnim = useRef(new Animated.Value(-DRAWER_WIDTH)).current;
  const fadeAnim = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (visible) {
      Animated.parallel([
        Animated.timing(slideAnim, {
          toValue: 0,
          duration: 240,
          useNativeDriver: true,
        }),
        Animated.timing(fadeAnim, {
          toValue: 1,
          duration: 240,
          useNativeDriver: true,
        }),
      ]).start();
    } else {
      Animated.parallel([
        Animated.timing(slideAnim, {
          toValue: -DRAWER_WIDTH,
          duration: 200,
          useNativeDriver: true,
        }),
        Animated.timing(fadeAnim, {
          toValue: 0,
          duration: 200,
          useNativeDriver: true,
        }),
      ]).start();
    }
  }, [visible, slideAnim, fadeAnim]);

  if (!visible) {
    return null;
  }

  const handleSelectTab = (targetMode: ViewMode) => {
    onSelectViewMode(targetMode);
    onClose();
  };

  const handleSelectSession = (sID: string) => {
    onSelectSession?.(sID);
    onClose();
  };

  const handleNewSession = () => {
    onNewSession?.();
    onClose();
  };

  const handleOpenSettings = () => {
    onOpenSettings();
    onClose();
  };

  return (
    <Modal
      animationType="none"
      onRequestClose={onClose}
      statusBarTranslucent
      transparent
      visible={visible}
    >
      <View style={styles.overlay}>
        {/* 点击暗色背景关闭抽屉 */}
        <Animated.View style={[styles.backdrop, { opacity: fadeAnim }]}>
          <Pressable onPress={onClose} style={StyleSheet.absoluteFill} />
        </Animated.View>

        {/* 侧滑功能抽屉本体 (借鉴图四) */}
        <Animated.View
          style={[
            styles.drawerSheet,
            {
              transform: [{ translateX: slideAnim }],
              width: DRAWER_WIDTH,
            },
          ]}
        >
          {/* 抽屉头部 */}
          <View style={styles.drawerHeader}>
            <View style={styles.brandGroup}>
              <View style={styles.avatarBox}>
                <Image
                  source={require("../../../assets/icon.png")}
                  style={styles.avatarImage}
                />
              </View>
              <View style={styles.brandInfo}>
                <Text style={styles.brandTitle}>MYAI Studio</Text>
                <Text numberOfLines={1} style={styles.brandSub}>
                  {connected
                    ? `${userID.trim() || "Mikasa"} · ${deviceID.trim() || "server-agent"}`
                    : "设备离线"}
                </Text>
              </View>
            </View>
            <Pressable
              accessibilityLabel="关闭菜单"
              onPress={onClose}
              style={({ pressed }) =>
                buttonFeedback(styles.closeButton, pressed)
              }
            >
              <Text style={styles.closeButtonText}>✕</Text>
            </Pressable>
          </View>

          <ScrollView
            contentContainerStyle={styles.scrollContent}
            showsVerticalScrollIndicator={false}
          >
            {/* 核心工作区收纳 (原底部 TabBar 收纳在此) */}
            <Text style={styles.sectionLabel}>功能工作区</Text>
            <View style={styles.navGroup}>
              <Pressable
                onPress={() => handleSelectTab("chat")}
                style={({ pressed }) =>
                  buttonFeedback(
                    [
                      styles.navItem,
                      viewMode === "chat" && styles.navItemActive,
                    ],
                    pressed,
                  )
                }
              >
                <View style={styles.navItemLeft}>
                  <Text style={styles.navIcon}>💬</Text>
                  <Text
                    style={[
                      styles.navText,
                      viewMode === "chat" && styles.navTextActive,
                    ]}
                  >
                    主对话流
                  </Text>
                </View>
                {viewMode === "chat" ? (
                  <View style={styles.currentBadge}>
                    <Text style={styles.currentBadgeText}>当前</Text>
                  </View>
                ) : null}
              </Pressable>

              <Pressable
                onPress={() => handleSelectTab("files")}
                style={({ pressed }) =>
                  buttonFeedback(
                    [
                      styles.navItem,
                      viewMode === "files" && styles.navItemActive,
                    ],
                    pressed,
                  )
                }
              >
                <View style={styles.navItemLeft}>
                  <Text style={styles.navIcon}>📁</Text>
                  <Text
                    style={[
                      styles.navText,
                      viewMode === "files" && styles.navTextActive,
                    ]}
                  >
                    工作区文件
                  </Text>
                </View>
              </Pressable>

              <Pressable
                onPress={() => handleSelectTab("changes")}
                style={({ pressed }) =>
                  buttonFeedback(
                    [
                      styles.navItem,
                      viewMode === "changes" && styles.navItemActive,
                    ],
                    pressed,
                  )
                }
              >
                <View style={styles.navItemLeft}>
                  <Text style={styles.navIcon}>⚡</Text>
                  <Text
                    style={[
                      styles.navText,
                      viewMode === "changes" && styles.navTextActive,
                    ]}
                  >
                    代码变更 (Git)
                  </Text>
                </View>
              </Pressable>

              <Pressable
                onPress={() => handleSelectTab("knowledge")}
                style={({ pressed }) =>
                  buttonFeedback(
                    [
                      styles.navItem,
                      viewMode === "knowledge" && styles.navItemActive,
                    ],
                    pressed,
                  )
                }
              >
                <View style={styles.navItemLeft}>
                  <Text style={styles.navIcon}>📚</Text>
                  <Text
                    style={[
                      styles.navText,
                      viewMode === "knowledge" && styles.navTextActive,
                    ]}
                  >
                    知识库与记忆
                  </Text>
                </View>
              </Pressable>
            </View>

            {/* 历史旁聊区 (会话列表) */}
            <View style={styles.sessionHeaderRow}>
              <Text style={styles.sectionLabel}>最近会话 (旁聊)</Text>
              <Pressable onPress={handleNewSession}>
                <Text style={styles.addSessionLink}>+ 新建</Text>
              </Pressable>
            </View>
            <View style={styles.sessionGroup}>
              {sessions.length > 0 ? (
                sessions.slice(0, 8).map((s) => {
                  const isCurrent = s.id === activeSessionID;
                  return (
                    <Pressable
                      key={s.id}
                      onPress={() => handleSelectSession(s.id)}
                      style={({ pressed }) =>
                        buttonFeedback(
                          [
                            styles.sessionCard,
                            isCurrent && styles.sessionCardActive,
                          ],
                          pressed,
                        )
                      }
                    >
                      <Text
                        numberOfLines={1}
                        style={[
                          styles.sessionTitle,
                          isCurrent && styles.sessionTitleActive,
                        ]}
                      >
                        {s.title?.trim() || "未命名会话"}
                      </Text>
                      <Text style={styles.sessionTime}>
                        {s.updated_at
                          ? new Date(s.updated_at).toLocaleDateString()
                          : "会话"}
                      </Text>
                    </Pressable>
                  );
                })
              ) : (
                <Text style={styles.emptySessionText}>暂无历史会话</Text>
              )}
            </View>
          </ScrollView>

          {/* 抽屉底部工具栏 (图四风格) */}
          <View style={styles.bottomBar}>
            <Pressable
              accessibilityLabel="设置中心"
              onPress={handleOpenSettings}
              style={({ pressed }) =>
                buttonFeedback(styles.toolButton, pressed)
              }
            >
              <Text style={styles.toolIcon}>⚙️</Text>
              <Text style={styles.toolLabel}>设置中心</Text>
            </Pressable>
            <Pressable
              accessibilityLabel="新建会话"
              onPress={handleNewSession}
              style={({ pressed }) =>
                buttonFeedback(
                  [styles.toolButton, styles.toolButtonPrimary],
                  pressed,
                )
              }
            >
              <Text style={styles.toolIcon}>✏️</Text>
              <Text style={[styles.toolLabel, styles.toolLabelPrimary]}>
                新对话
              </Text>
            </Pressable>
          </View>
        </Animated.View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  overlay: {
    flex: 1,
  },
  backdrop: {
    ...StyleSheet.absoluteFill,
    backgroundColor: "rgba(18, 16, 14, 0.45)",
  },
  drawerSheet: {
    backgroundColor: "#faf5ec",
    borderRightColor: "#12100e",
    borderRightWidth: 3,
    bottom: 0,
    elevation: 16,
    left: 0,
    paddingBottom: 20,
    paddingTop: 48,
    position: "absolute",
    shadowColor: "#000",
    shadowOffset: { height: 0, width: 6 },
    shadowOpacity: 0.25,
    shadowRadius: 16,
    top: 0,
  },
  drawerHeader: {
    alignItems: "center",
    borderBottomColor: "#12100e",
    borderBottomWidth: 2,
    flexDirection: "row",
    justifyContent: "space-between",
    marginHorizontal: 16,
    paddingBottom: 14,
  },
  brandGroup: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
  },
  avatarBox: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 12,
    borderWidth: 2,
    elevation: 2,
    height: 40,
    overflow: "hidden",
    shadowColor: "#12100e",
    shadowOffset: { height: 1.5, width: 1.5 },
    shadowOpacity: 1,
    shadowRadius: 0,
    width: 40,
  },
  avatarImage: {
    height: "100%",
    width: "100%",
  },
  brandInfo: {
    justifyContent: "center",
  },
  brandTitle: {
    color: "#12100e",
    fontSize: 16,
    fontWeight: "900",
  },
  brandSub: {
    color: "#726c62",
    fontSize: 11,
    fontWeight: "800",
    marginTop: 2,
  },
  closeButton: {
    alignItems: "center",
    backgroundColor: "#efe7d8",
    borderColor: "#12100e",
    borderRadius: 16,
    borderWidth: 1.5,
    height: 32,
    justifyContent: "center",
    width: 32,
  },
  closeButtonText: {
    color: "#12100e",
    fontSize: 14,
    fontWeight: "900",
  },
  scrollContent: {
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  sectionLabel: {
    color: "#726c62",
    fontSize: 11,
    fontWeight: "900",
    letterSpacing: 0.5,
    marginBottom: 8,
    marginTop: 4,
    textTransform: "uppercase",
  },
  navGroup: {
    gap: 6,
    marginBottom: 16,
  },
  navItem: {
    alignItems: "center",
    backgroundColor: "transparent",
    borderColor: "transparent",
    borderRadius: 14,
    borderWidth: 2,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  navItemActive: {
    backgroundColor: "#ffffff",
    borderColor: "#12100e",
    elevation: 2,
    shadowColor: "#12100e",
    shadowOffset: { height: 1.5, width: 1.5 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  navItemLeft: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
  },
  navIcon: {
    fontSize: 16,
    width: 22,
  },
  navText: {
    color: "#25231f",
    fontSize: 13.5,
    fontWeight: "800",
  },
  navTextActive: {
    color: "#12100e",
    fontWeight: "900",
  },
  currentBadge: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderRadius: 999,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  currentBadgeText: {
    color: "#12100e",
    fontSize: 10,
    fontWeight: "900",
  },
  sessionHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  addSessionLink: {
    color: "#12100e",
    fontSize: 11,
    fontWeight: "900",
  },
  sessionGroup: {
    gap: 6,
    marginTop: 4,
  },
  sessionCard: {
    backgroundColor: "#fefcf7",
    borderColor: "#e4dbcc",
    borderRadius: 10,
    borderWidth: 1.5,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  sessionCardActive: {
    backgroundColor: "#ffd84f",
    borderColor: "#12100e",
    borderWidth: 1.5,
    elevation: 1.5,
    shadowColor: "#12100e",
    shadowOffset: { height: 1, width: 1 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  sessionTitle: {
    color: "#25231f",
    fontSize: 12,
    fontWeight: "800",
  },
  sessionTitleActive: {
    color: "#12100e",
    fontWeight: "900",
  },
  sessionTime: {
    color: "#726c62",
    fontSize: 10,
    marginTop: 2,
  },
  emptySessionText: {
    color: "#8a8377",
    fontSize: 11,
    fontStyle: "italic",
    paddingVertical: 8,
  },
  bottomBar: {
    borderTopColor: "#12100e",
    borderTopWidth: 2,
    flexDirection: "row",
    gap: 10,
    marginTop: "auto",
    paddingHorizontal: 16,
    paddingTop: 12,
  },
  toolButton: {
    alignItems: "center",
    backgroundColor: "#ffffff",
    borderColor: "#12100e",
    borderRadius: 12,
    borderWidth: 2,
    elevation: 2,
    flex: 1,
    flexDirection: "row",
    gap: 6,
    height: 40,
    justifyContent: "center",
    shadowColor: "#12100e",
    shadowOffset: { height: 1.5, width: 1.5 },
    shadowOpacity: 1,
    shadowRadius: 0,
  },
  toolButtonPrimary: {
    backgroundColor: "#ffd84f",
  },
  toolIcon: {
    fontSize: 15,
  },
  toolLabel: {
    color: "#12100e",
    fontSize: 12,
    fontWeight: "900",
  },
  toolLabelPrimary: {
    color: "#12100e",
  },
});
