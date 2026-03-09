interface ModalProps {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  footer?: React.ReactNode;
  maxWidth?: number;
  variant?: 'default' | 'user';
  bodyStyle?: React.CSSProperties;
}

export default function Modal({ title, onClose, children, footer, maxWidth, variant = 'default', bodyStyle }: ModalProps) {
  const prefix = variant === 'user' ? 'user-modal' : 'modal';

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className={prefix} style={maxWidth ? { maxWidth } : undefined} onClick={e => e.stopPropagation()}>
        <div className={`${prefix}-header`}>
          <h3>{title}</h3>
          <button className={variant === 'user' ? 'user-modal-close' : 'btn-clear'} onClick={onClose}>
            {variant === 'user' ? '\u00d7' : 'X'}
          </button>
        </div>
        <div className={`${prefix}-body`} style={bodyStyle}>
          {children}
        </div>
        {footer && (
          <div className={`${prefix}-footer`}>
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}
