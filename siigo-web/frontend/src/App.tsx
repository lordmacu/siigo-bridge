import { Routes, Route } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Dashboard from './pages/Dashboard';
import DataPage from './pages/DataPage';
import Logs from './pages/Logs';
import Config from './pages/Config';
import ToastContainer from './components/Toast';

export default function App() {
  return (
    <div className="app-layout">
      <Sidebar />
      <div className="main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/clients" element={<DataPage table="clients" title="Clientes (Z17 - Terceros)" file="Z17" />} />
          <Route path="/products" element={<DataPage table="products" title="Productos (Z06CP - Inventario)" file="Z06CP" />} />
          <Route path="/movements" element={<DataPage table="movements" title="Movimientos (Z49 - Transacciones)" file="Z49" />} />
          <Route path="/cartera" element={<DataPage table="cartera" title="Cartera (Z09 - Cuentas por Cobrar)" file="Z09" />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/config" element={<Config />} />
        </Routes>
      </div>
      <ToastContainer />
    </div>
  );
}
