interface LoadingStateProps {
  message?: string;
}

const LoadingState = ({ message = "加载中，请稍候..." }: LoadingStateProps) => (
  <div className="flex h-32 flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-slate-700 text-slate-300">
    <svg className="h-6 w-6 animate-spin text-primary" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
    </svg>
    <span>{message}</span>
  </div>
);

export default LoadingState;
