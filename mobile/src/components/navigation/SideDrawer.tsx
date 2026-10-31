import { useRef, useEffect } from "react";
import {
  Animated,
  Dimensions,
  Image,
  Modal,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { Feather } from "@expo/vector-icons";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type { SessionSummary } from "../../protocol";
import type { ViewMode } from "../../types/app";
import type { ButtonFeedback } from "../../types/ui";
import { font, palette, radius, shadow, weight } from "../../constants/theme";

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
  const insets = useSafeAreaInsets();
  const topSafe = Math.max(insets.top, Platform.OS === "android" ? 24 : 16);
  const bottomSafe = Math.max(insets.bottom, Platform.OS === "android" ? 28 : 16);
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

        {/* 侧滑功能抽屉本体 */}
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
          <View style={[styles.drawerHeader, { paddingTop: topSafe + 8 }]}>
            <View style={styles.brandGroup}>
              <View style={styles.avatarBox}>
                <Image
                  source={require("../../../assets/avatar.png")}
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
              <Feather color={palette.ink2} name="x" size={18} />
            </Pressable>
          </View>

          <ScrollView
            contentContainerStyle={styles.scrollContent}
            showsVerticalScrollIndicator={false}
          >
            {/* 核心工作区收纳 */}
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
                  <Feather
                    color={viewMode === "chat" ? palette.brand : palette.ink2}
                    name="message-circle"
                    size={18}
                  />
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
                  <Feather
                    color={viewMode === "files" ? palette.brand : palette.ink2}
                    name="folder"
                    size={18}
                  />
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
                  <Feather
                    color={viewMode === "changes" ? palette.brand : palette.ink2}
                    name="zap"
                    size={18}
                  />
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
                  <Feather
                    color={viewMode === "knowledge" ? palette.brand : palette.ink2}
                    name="book-open"
                    size={18}
                  />
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

          {/* 抽屉底部工具栏 */}
          <View style={[styles.bottomBar, { paddingBottom: bottomSafe + 8 }]}>
            <Pressable
              accessibilityLabel="设置中心"
              onPress={handleOpenSettings}
              style={({ pressed }) =>
                buttonFeedback(styles.toolButton, pressed)
              }
            >
              <Feather color={palette.ink2} name="settings" size={18} />
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
              <Feather color={palette.inkInverse} name="edit-2" size={18} />
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
    backgroundColor: palette.scrim,
  },
  drawerSheet: {
    backgroundColor: palette.surface1,
    borderRightColor: palette.hairline,
    borderRightWidth: 1,
    bottom: 0,
    left: 0,
    paddingBottom: 20,
    paddingTop: 48,
    position: "absolute",
    top: 0,
    ...shadow.float,
  },
  drawerHeader: {
    alignItems: "center",
    borderBottomColor: palette.hairlineSoft,
    borderBottomWidth: 1,
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
    backgroundColor: palette.surface3,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    height: 42,
    overflow: "hidden",
    width: 42,
  },
  avatarImage: {
    height: "100%",
    width: "100%",
  },
  brandInfo: {
    justifyContent: "center",
  },
  brandTitle: {
    color: palette.ink1,
    fontSize: font.title,
    fontWeight: weight.bold,
  },
  brandSub: {
    color: palette.ink2,
    fontSize: font.caption,
    fontWeight: weight.medium,
    marginTop: 2,
  },
  closeButton: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.lg,
    borderWidth: 1,
    height: 32,
    justifyContent: "center",
    width: 32,
  },
  scrollContent: {
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  sectionLabel: {
    color: palette.ink3,
    fontSize: font.caption,
    fontWeight: weight.bold,
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
    borderRadius: radius.md,
    borderWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  navItemActive: {
    backgroundColor: palette.brandWash,
    borderColor: palette.hairline,
  },
  navItemLeft: {
    alignItems: "center",
    flexDirection: "row",
    gap: 10,
  },
  navText: {
    color: palette.ink2,
    fontSize: font.body,
    fontWeight: weight.semibold,
  },
  navTextActive: {
    color: palette.ink1,
    fontWeight: weight.bold,
  },
  currentBadge: {
    backgroundColor: palette.brandWashStrong,
    borderColor: palette.brandDeep,
    borderRadius: radius.pill,
    borderWidth: 1,
    paddingHorizontal: 6,
    paddingVertical: 1,
  },
  currentBadgeText: {
    color: palette.brand,
    fontSize: font.micro,
    fontWeight: weight.bold,
  },
  sessionHeaderRow: {
    alignItems: "center",
    flexDirection: "row",
    justifyContent: "space-between",
  },
  addSessionLink: {
    color: palette.brand,
    fontSize: font.caption,
    fontWeight: weight.bold,
  },
  sessionGroup: {
    gap: 6,
    marginTop: 4,
  },
  sessionCard: {
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  sessionCardActive: {
    backgroundColor: palette.brandWash,
    borderColor: palette.brandDeep,
    borderWidth: 1,
  },
  sessionTitle: {
    color: palette.ink2,
    fontSize: font.label,
    fontWeight: weight.semibold,
  },
  sessionTitleActive: {
    color: palette.ink1,
    fontWeight: weight.bold,
  },
  sessionTime: {
    color: palette.ink3,
    fontSize: font.micro,
    marginTop: 2,
  },
  emptySessionText: {
    color: palette.ink3,
    fontSize: font.caption,
    fontStyle: "italic",
    paddingVertical: 8,
  },
  bottomBar: {
    borderTopColor: palette.hairlineSoft,
    borderTopWidth: 1,
    flexDirection: "row",
    gap: 10,
    marginTop: "auto",
    paddingHorizontal: 16,
    paddingTop: 12,
  },
  toolButton: {
    alignItems: "center",
    backgroundColor: palette.surface2,
    borderColor: palette.hairline,
    borderRadius: radius.md,
    borderWidth: 1,
    flex: 1,
    flexDirection: "row",
    gap: 6,
    height: 40,
    justifyContent: "center",
  },
  toolButtonPrimary: {
    backgroundColor: palette.brand,
    borderColor: palette.brandDeep,
  },
  toolLabel: {
    color: palette.ink1,
    fontSize: font.label,
    fontWeight: weight.bold,
  },
  toolLabelPrimary: {
    color: palette.inkInverse,
  },
});
