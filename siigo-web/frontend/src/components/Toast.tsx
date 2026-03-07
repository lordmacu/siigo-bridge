import { useState, useEffect, useCallback } from 'react';

type ToastType = 'success' | 'error';
interface ToastData { type: ToastType; message: string; id: number }

let addToastFn: (type: ToastType, message: string) => void = () => {};

export function showToast(type: ToastType, message: string) {
  addToastFn(type, message);
}

let nextId = 0;

export default function ToastContainer() {
  const [toasts, setToasts] = useState<ToastData[]>([]);

  const addToast = useCallback((type: ToastType, message: string) => {
    const id = nextId++;
    setToasts(prev => [...prev, { type, message, id }]);
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, 3000);
  }, []);

  useEffect(() => {
    addToastFn = addToast;
  }, [addToast]);

  return (
    <div className="toast-container">
      {toasts.map(t => (
        <div key={t.id} className={`toast ${t.type}`}>{t.message}</div>
      ))}
    </div>
  );
}
