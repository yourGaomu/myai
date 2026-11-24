import { useCallback, useState } from "react";

import type {
  ImageModelConfigAddResultPayload,
  ImageModelConfigMutationResultPayload,
  ImageModelConfigTestResultPayload,
  ImageModelListResultPayload,
  ImageModelSummary,
} from "../protocol";

export function useImageModelState() {
  const [models, setModels] = useState<ImageModelSummary[]>([]);
  const [defaultModelID, setDefaultModelID] = useState("");
  const [message, setMessage] = useState("");
  const [messageError, setMessageError] = useState(false);
  const [testResult, setTestResult] = useState<ImageModelConfigTestResultPayload | null>(null);

  const applyList = useCallback((payload?: ImageModelListResultPayload) => {
    setModels(payload?.models || []);
    setDefaultModelID(payload?.default_model_id || "");
    setMessage("");
    setMessageError(false);
    setTestResult(null);
  }, []);
  const applyAdd = useCallback((payload?: ImageModelConfigAddResultPayload) => {
    if (payload?.models) setModels(payload.models);
    if (payload?.default_model_id !== undefined) setDefaultModelID(payload.default_model_id);
    setMessage("图片模型已添加。");
    setMessageError(false);
    setTestResult(null);
  }, []);
  const applyTest = useCallback((payload?: ImageModelConfigTestResultPayload) => {
    if (!payload) return;
    setTestResult(payload);
    setMessage(payload.message || (payload.ok ? "连接测试成功。" : "连接测试失败。"));
    setMessageError(!payload.ok);
  }, []);
  const applyMutation = useCallback((payload?: ImageModelConfigMutationResultPayload) => {
    if (payload?.models) setModels(payload.models);
    if (payload?.default_model_id !== undefined) setDefaultModelID(payload.default_model_id);
    setMessage(payload?.message || "图片模型配置已更新。");
    setMessageError(false);
    setTestResult(null);
  }, []);
  const applyError = useCallback((text: string) => {
    setMessage(text || "图片模型操作失败，请检查连接和配置后重试。");
    setMessageError(true);
    setTestResult(null);
  }, []);
  const clearMessage = useCallback(() => {
    setMessage("");
    setMessageError(false);
    setTestResult(null);
  }, []);
  const clear = useCallback(() => {
    setModels([]);
    setDefaultModelID("");
    clearMessage();
  }, [clearMessage]);

  return { applyAdd, applyError, applyList, applyMutation, applyTest, clear, clearMessage, defaultModelID, message, messageError, models, testResult };
}
