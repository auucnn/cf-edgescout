interface ErrorStateProps {
  error: Error | null;
  retry?: () => void;
}

const ErrorState = ({ error, retry }: ErrorStateProps) => (
  <div className="flex h-32 flex-col items-center justify-center gap-2 rounded-lg border border-red-600/50 bg-red-500/10 text-red-200">
    <span>加载失败：{error?.message ?? "未知错误"}</span>
    {retry && (
      <button
        onClick={retry}
        className="rounded-md border border-red-400 px-3 py-1 text-sm hover:bg-red-400/10"
        type="button"
      >
        重试
      </button>
    )}
  </div>
);

export default ErrorState;
